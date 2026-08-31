// Package binggrounding adapts a dedicated Microsoft Foundry Toolbox Web
// Search tool. It never calls the retired Bing Search APIs and never treats a
// model-synthesized answer as original page content.
package binggrounding

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/domain"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/evidencecapture"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/sourcenet"
)

const (
	sourceCode              = "bing_grounding"
	collectorProfileVersion = "bing-foundry-mcp-response-v1"
	toolName                = "web_search"
	featureHeader           = "Toolboxes=V1Preview"
	protocolVersion         = "2025-03-26"
	maxQueryCharacters      = 1024
	maxResponseBodyBytes    = 4 << 20
	maxRedirects            = 3
	modelGeneratedTitle     = "Microsoft Foundry Web Search（模型生成的派生证据）"
	modelGeneratedAuthor    = "Microsoft Foundry Web Search（模型生成）"
)

var (
	errUnsafeDestination = errors.New("unsafe Foundry destination")
	errRedirectLimit     = errors.New("foundry redirect limit exceeded")
	emailPattern         = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	secretPattern        = regexp.MustCompile(`(?i)\b(?:api[_ -]?key|access[_ -]?token|password|secret|bearer)\s*[:=]\s*\S+`)
	jwtPattern           = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

type connectorOptions struct {
	resolver    lookupIPAddrFunc
	dialContext func(context.Context, string, string) (net.Conn, error)
	tlsConfig   *tls.Config
	now         func() time.Time
	lookupEnv   func(string) (string, bool)
}

type Connector struct {
	sourceID      int64
	endpoint      *url.URL
	credentialRef string
	approved      bool
	enabled       bool
	deleted       bool
	http          *http.Client
	now           func() time.Time
	lookupEnv     func(string) (string, bool)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code int `json:"code"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type toolListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpTool struct {
	Name        string `json:"name"`
	InputSchema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	} `json:"inputSchema"`
}

type toolCallResult struct {
	IsError bool         `json:"isError"`
	Content []mcpContent `json:"content"`
}

type mcpContent struct {
	Type     string       `json:"type"`
	Text     string       `json:"text"`
	Resource *mcpResource `json:"resource"`
}

type mcpResource struct {
	Text string        `json:"text"`
	Meta groundingMeta `json:"_meta"`
}

type groundingMeta struct {
	Annotations []citation `json:"annotations"`
	Action      struct {
		Query   string   `json:"query"`
		Queries []string `json:"queries"`
	} `json:"action"`
}

type fetchedRPCResponse struct {
	rawPayload    []byte
	selectedStart int64
	selectedEnd   int64
	statusCode    int
	requestedURL  string
	finalURL      string
	redirectChain []string
	headers       http.Header
	capturedAt    time.Time
}

func (value fetchedRPCResponse) snapshot() (domain.EvidenceSnapshot, error) {
	return evidencecapture.NewHTTPSnapshot(value.rawPayload, "text/event-stream", collectorProfileVersion, value.requestedURL, value.finalURL,
		value.redirectChain, value.statusCode, value.headers, value.capturedAt)
}

type citation struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func New(connection domain.SourceConnection, resolvers ...sourcenet.Resolver) (*Connector, error) {
	options := connectorOptions{}
	if len(resolvers) > 0 && resolvers[0] != nil {
		options.resolver = resolvers[0].LookupIPAddr
	}
	return newConnector(connection, options)
}

func NewWithCredentialLookup(connection domain.SourceConnection, resolver sourcenet.Resolver, lookup func(string) (string, bool)) (*Connector, error) {
	options := connectorOptions{lookupEnv: lookup}
	if resolver != nil {
		options.resolver = resolver.LookupIPAddr
	}
	return newConnector(connection, options)
}

func newConnector(connection domain.SourceConnection, options connectorOptions) (*Connector, error) {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeBingGrounding {
		return nil, permanent("invalid Bing Grounding source connection")
	}
	endpoint, err := url.Parse(normalized.Endpoint)
	if err != nil || !sameEndpoint(endpoint, endpoint) {
		return nil, permanent("invalid Foundry Toolbox endpoint")
	}
	if options.resolver == nil {
		options.resolver = net.DefaultResolver.LookupIPAddr
	}
	if options.dialContext == nil {
		options.dialContext = (&net.Dialer{}).DialContext
	}
	if options.now == nil {
		options.now = func() time.Time { return time.Now().UTC() }
	}
	if options.lookupEnv == nil {
		options.lookupEnv = os.LookupEnv
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if options.tlsConfig != nil {
		tlsConfig = options.tlsConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	timeout := time.Duration(normalized.Config.RequestTimeoutSeconds) * time.Second
	transport := &http.Transport{
		Proxy: nil, ForceAttemptHTTP2: true, TLSClientConfig: tlsConfig,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: timeout,
		DialContext: secureDialContext(options.resolver, options.dialContext),
	}
	client := &http.Client{
		Timeout: timeout, Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errRedirectLimit
			}
			if !sameEndpoint(endpoint, request.URL) {
				return errUnsafeDestination
			}
			return nil
		},
	}
	return &Connector{
		sourceID: normalized.ID, endpoint: endpoint, credentialRef: normalized.CredentialRef,
		approved: normalized.Config.GroundingDataBoundaryApproved,
		enabled:  normalized.Enabled, deleted: normalized.Deleted, http: client,
		now: options.now, lookupEnv: options.lookupEnv,
	}, nil
}

func (connector *Connector) Validate(_ context.Context, connection domain.SourceConnection) error {
	normalized, err := domain.NormalizeSourceConnection(connection)
	if err != nil || normalized.SourceType != domain.SourceTypeBingGrounding ||
		(connector.sourceID > 0 && normalized.ID != connector.sourceID) ||
		normalized.Endpoint != connector.endpoint.String() || normalized.CredentialRef != connector.credentialRef ||
		normalized.Config.GroundingDataBoundaryApproved != connector.approved {
		return permanent("Bing Grounding source connection does not match connector")
	}
	return nil
}

func (connector *Connector) Fetch(ctx context.Context, request domain.FetchRequest) (domain.FetchResult, error) {
	result := domain.FetchResult{Items: []domain.SourceItem{}, Diagnostics: []domain.FetchDiagnostic{}}
	if err := request.Validate(); err != nil || (connector.sourceID > 0 && request.SourceConnectionID != connector.sourceID) || strings.TrimSpace(request.RequestCursor) != "" {
		return result, permanent("invalid Bing Grounding fetch request")
	}
	if !connector.enabled || connector.deleted {
		return result, permanent("Bing Grounding source connection is unavailable")
	}
	if !connector.approved {
		return result, permanent("Bing Grounding data boundary review is required")
	}
	query, err := normalizeQuery(request.Query)
	if err != nil {
		return result, permanent("invalid Bing Grounding search query")
	}
	token, err := connector.token()
	if err != nil {
		return result, err
	}
	session, selectedTool, err := connector.initialize(ctx, token)
	if err != nil {
		return result, err
	}
	envelope, captured, err := connector.postRPCCaptured(ctx, token, session, rpcRequest{
		JSONRPC: "2.0", ID: 3, Method: "tools/call",
		Params: map[string]any{"name": selectedTool, "arguments": map[string]string{"search_query": query}},
	})
	if err != nil {
		return result, err
	}
	var call toolCallResult
	if envelope.Error != nil || len(envelope.Result) == 0 || json.Unmarshal(envelope.Result, &call) != nil || call.IsError {
		return result, parse("invalid Foundry Web Search response")
	}
	item, err := connector.mapResult(call, request.QuerySignature, query, captured.capturedAt)
	if err != nil {
		return result, parse("invalid Foundry Web Search evidence")
	}
	snapshot, err := captured.snapshot()
	if err != nil {
		return result, parse("capture Foundry Web Search response")
	}
	selected := captured.rawPayload[captured.selectedStart:captured.selectedEnd]
	selectedDigest := sha256.Sum256(selected)
	start, end := captured.selectedStart, captured.selectedEnd
	item.EvidenceReferences = []domain.EvidenceReference{{
		SnapshotKey: snapshot.Key, Usage: domain.EvidenceUsageDocumentSource, LocatorType: domain.EvidenceLocatorByteRange,
		LocatorValue: "bytes[" + strconv.FormatInt(start, 10) + ":" + strconv.FormatInt(end, 10) + "]",
		ByteStart:    &start, ByteEnd: &end, SelectedPayloadSHA256: hex.EncodeToString(selectedDigest[:]),
		SelectorVersion: domain.ByteRangeSelectorVersion,
	}}
	item.SnapshotKey = snapshot.Key
	item.ItemLocator = item.EvidenceReferences[0].LocatorValue
	item, err = domain.NormalizeSourceItem(item)
	if err != nil {
		return result, parse("normalize Foundry Web Search evidence")
	}
	result.Snapshots = append(result.Snapshots, snapshot)
	result.Items = append(result.Items, item)
	return result, nil
}

func (connector *Connector) Health(ctx context.Context, connection domain.SourceConnection) domain.HealthResult {
	checkedAt := connector.now().UTC()
	if err := connector.Validate(ctx, connection); err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "invalid_source_connection"}
	}
	if connector.deleted {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "invalid_source_connection"}
	}
	if !connector.approved {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorPermanent, DiagnosticCode: "data_boundary_review_required"}
	}
	token, err := connector.token()
	if err != nil {
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.CollectionErrorAuthentication, DiagnosticCode: "credential_unavailable"}
	}
	if _, _, err := connector.initialize(ctx, token); err != nil {
		code := "request_failed"
		switch domain.ClassifyCollectionError(err) {
		case domain.CollectionErrorAuthentication:
			code = "credential_unavailable"
		case domain.CollectionErrorParse, domain.CollectionErrorPermanent:
			code = "toolbox_contract_invalid"
		}
		return domain.HealthResult{CheckedAt: checkedAt, ErrorKind: domain.ClassifyCollectionError(err), DiagnosticCode: code}
	}
	return domain.HealthResult{Healthy: true, CheckedAt: checkedAt}
}

func (connector *Connector) initialize(ctx context.Context, token string) (string, string, error) {
	envelope, session, _, err := connector.post(ctx, token, "", rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "initialize",
		Params: map[string]any{
			"protocolVersion": protocolVersion, "capabilities": map[string]any{},
			"clientInfo": map[string]string{"name": "hotkey", "version": "1.0"},
		},
	}, true)
	if err != nil {
		return "", "", err
	}
	var initialized initializeResult
	if envelope.Error != nil || session == "" || json.Unmarshal(envelope.Result, &initialized) != nil || initialized.ProtocolVersion == "" {
		return "", "", parse("invalid Foundry MCP initialize response")
	}
	if err := connector.notifyInitialized(ctx, token, session); err != nil {
		return "", "", err
	}
	listed, err := connector.postRPC(ctx, token, session, rpcRequest{JSONRPC: "2.0", ID: 2, Method: "tools/list", Params: map[string]any{}})
	if err != nil {
		return "", "", err
	}
	var tools toolListResult
	if listed.Error != nil || json.Unmarshal(listed.Result, &tools) != nil || len(tools.Tools) != 1 || !validWebSearchTool(tools.Tools[0]) {
		return "", "", parse("Foundry Toolbox must expose one Web Search tool")
	}
	return session, tools.Tools[0].Name, nil
}

func (connector *Connector) notifyInitialized(ctx context.Context, token, session string) error {
	payload, err := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"})
	if err != nil {
		return permanent("encode Foundry initialized notification")
	}
	request, err := connector.request(ctx, token, session, payload)
	if err != nil {
		return err
	}
	response, err := connector.http.Do(request)
	if err != nil {
		return requestError(err)
	}
	status := response.StatusCode
	closeResponse(response)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return statusError(status)
	}
	return nil
}

func (connector *Connector) postRPC(ctx context.Context, token, session string, value rpcRequest) (rpcEnvelope, error) {
	envelope, _, _, err := connector.post(ctx, token, session, value, false)
	return envelope, err
}

func (connector *Connector) postRPCCaptured(ctx context.Context, token, session string, value rpcRequest) (rpcEnvelope, fetchedRPCResponse, error) {
	envelope, _, captured, err := connector.post(ctx, token, session, value, false)
	return envelope, captured, err
}

func (connector *Connector) post(ctx context.Context, token, session string, value rpcRequest, captureSession bool) (rpcEnvelope, string, fetchedRPCResponse, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, permanent("encode Foundry MCP request")
	}
	request, err := connector.request(ctx, token, session, payload)
	if err != nil {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, err
	}
	response, err := connector.http.Do(request)
	if err != nil {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, requestError(err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		status := response.StatusCode
		closeResponse(response)
		return rpcEnvelope{}, "", fetchedRPCResponse{}, statusError(status)
	}
	responsePayload, selectedStart, selectedEnd, rawPayload, err := readPayload(response)
	if err != nil {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, err
	}
	var envelope rpcEnvelope
	if json.Unmarshal(responsePayload, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != value.ID || (len(envelope.Result) == 0 && envelope.Error == nil) {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, parse("decode Foundry MCP response")
	}
	if envelope.Error != nil {
		return rpcEnvelope{}, "", fetchedRPCResponse{}, rpcStatusError(envelope.Error.Code)
	}
	responseSession := session
	if captureSession {
		responseSession = strings.TrimSpace(response.Header.Get("Mcp-Session-Id"))
	}
	return envelope, responseSession, fetchedRPCResponse{
		rawPayload: rawPayload, selectedStart: selectedStart, selectedEnd: selectedEnd,
		statusCode: response.StatusCode, requestedURL: request.URL.String(), finalURL: response.Request.URL.String(),
		redirectChain: evidencecapture.RedirectChain(request.URL.String(), response.Request), headers: response.Header.Clone(), capturedAt: connector.now().UTC(),
	}, nil
}

func (connector *Connector) request(ctx context.Context, token, session string, payload []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, connector.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, permanent("create Foundry MCP request")
	}
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Foundry-Features", featureHeader)
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
	}
	return request, nil
}

func readPayload(response *http.Response) ([]byte, int64, int64, []byte, error) {
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBodyBytes+1))
	if err != nil || len(payload) > maxResponseBodyBytes {
		return nil, 0, 0, nil, permanent("Foundry response exceeds the safe body limit")
	}
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		selected, err := sseJSON(payload)
		if err != nil {
			return nil, 0, 0, nil, err
		}
		start := bytes.LastIndex(payload, selected)
		if start < 0 {
			return nil, 0, 0, nil, parse("locate Foundry event stream payload")
		}
		return selected, int64(start), int64(start + len(selected)), payload, nil
	}
	return payload, 0, int64(len(payload)), payload, nil
}

func sseJSON(payload []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 64*1024), maxResponseBodyBytes)
	var data []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if value != "" && value != "[DONE]" {
				data = append(data, value)
			}
		}
	}
	if scanner.Err() != nil || len(data) == 0 {
		return nil, parse("decode Foundry event stream")
	}
	// A Streamable HTTP response may contain progress events. The final JSON-RPC
	// envelope is authoritative and is therefore the last non-DONE data event.
	return []byte(data[len(data)-1]), nil
}

func validWebSearchTool(tool mcpTool) bool {
	if tool.Name != toolName || tool.InputSchema.Type != "object" {
		return false
	}
	if _, ok := tool.InputSchema.Properties["search_query"]; !ok {
		return false
	}
	for _, required := range tool.InputSchema.Required {
		if required == "search_query" {
			return true
		}
	}
	return false
}

func (connector *Connector) mapResult(result toolCallResult, querySignature, query string, observedAt time.Time) (domain.SourceItem, error) {
	var answer string
	var annotations []citation
	queryReference := query
	for _, content := range result.Content {
		if content.Type == "resource" && content.Resource != nil && strings.TrimSpace(content.Resource.Text) != "" {
			if answer != "" {
				return domain.SourceItem{}, errors.New("multiple Web Search resources")
			}
			answer = content.Resource.Text
			annotations = content.Resource.Meta.Annotations
			if value := strings.TrimSpace(content.Resource.Meta.Action.Query); value != "" {
				queryReference = value
			} else if len(content.Resource.Meta.Action.Queries) > 0 && strings.TrimSpace(content.Resource.Meta.Action.Queries[0]) != "" {
				queryReference = strings.TrimSpace(content.Resource.Meta.Action.Queries[0])
			}
		}
	}
	if strings.TrimSpace(answer) == "" {
		return domain.SourceItem{}, errors.New("web Search answer is missing")
	}
	queryReference, err := normalizeQuery(queryReference)
	if err != nil {
		return domain.SourceItem{}, errors.New("web Search query reference is invalid")
	}
	attachments := make([]domain.SourceAttachment, 0, len(annotations))
	seen := make(map[string]struct{}, len(annotations))
	for _, annotation := range annotations {
		rawURL := strings.TrimSpace(annotation.URL)
		parsed, err := url.Parse(rawURL)
		if annotation.Type != "url_citation" || err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			continue
		}
		if _, exists := seen[rawURL]; exists {
			continue
		}
		seen[rawURL] = struct{}{}
		attachments = append(attachments, domain.SourceAttachment{URL: rawURL})
		if len(attachments) == domain.MaxSourceAttachments {
			break
		}
	}
	if len(attachments) == 0 {
		return domain.SourceItem{}, errors.New("web Search citations are missing")
	}
	digestInput := querySignature + "\x00" + query + "\x00" + answer
	for _, attachment := range attachments {
		digestInput += "\x00" + attachment.URL
	}
	digest := sha256.Sum256([]byte(digestInput))
	return domain.NormalizeSourceItem(domain.SourceItem{
		SourceCode: sourceCode, ExternalID: hex.EncodeToString(digest[:]), ContentType: "bulletin",
		Title: modelGeneratedTitle + " · 查询：" + queryReference, URL: attachments[0].URL, Author: modelGeneratedAuthor,
		ObservedAt: observedAt.UTC(), EvidenceCompleteness: domain.EvidenceCompletenessMetadataOnly,
		Attachments: attachments, Metrics: domain.SourceMetrics{},
		Parties: []domain.SourcePartyAssertion{{
			Role: domain.SourcePartyRoleDistributor, Kind: domain.SourcePartyKindOrganization,
			IdentityNamespace: "platform", ExternalID: "microsoft-bing-grounding", DisplayName: "Microsoft Foundry Web Search",
			HomepageURL: "https://learn.microsoft.com/azure/ai-foundry/agents/how-to/tools/bing-grounding",
		}},
	})
}

func normalizeQuery(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > maxQueryCharacters || emailPattern.MatchString(value) || secretPattern.MatchString(value) || jwtPattern.MatchString(value) {
		return "", errors.New("unsafe Grounding query")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", errors.New("unsafe Grounding query")
		}
	}
	return value, nil
}

func (connector *Connector) token() (string, error) {
	name := strings.TrimPrefix(connector.credentialRef, "env:")
	token, ok := connector.lookupEnv(name)
	if !ok || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return "", domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("foundry credential is unavailable"))
	}
	return token, nil
}

func sameEndpoint(endpoint, candidate *url.URL) bool {
	return endpoint != nil && candidate != nil && candidate.Scheme == "https" &&
		strings.EqualFold(candidate.Hostname(), endpoint.Hostname()) &&
		(candidate.Port() == "" || candidate.Port() == "443") && candidate.Path == endpoint.Path && candidate.RawQuery == endpoint.RawQuery
}

func secureDialContext(resolver lookupIPAddrFunc, dialContext func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || network != "tcp" || port != "443" {
			return nil, errUnsafeDestination
		}
		addresses, err := resolver(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errUnsafeDestination
		}
		for _, address := range addresses {
			if !publicAddress(address.IP) {
				return nil, errUnsafeDestination
			}
		}
		var dialErr error
		for _, address := range addresses {
			connection, err := dialContext(ctx, network, net.JoinHostPort(address.IP.String(), port))
			if err == nil {
				return connection, nil
			}
			dialErr = err
		}
		if dialErr != nil {
			return nil, dialErr
		}
		return nil, errUnsafeDestination
	}
}

func publicAddress(value net.IP) bool {
	address, ok := netip.AddrFromSlice(value)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, blocked := range blockedAddressRanges {
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

var blockedAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func requestError(err error) error {
	if errors.Is(err, errUnsafeDestination) || errors.Is(err, errRedirectLimit) {
		return permanent("Foundry destination is not permitted")
	}
	return temporary("Foundry request failed")
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("foundry authentication failed"))
	case http.StatusTooManyRequests:
		return domain.NewCollectionError(domain.CollectionErrorRateLimited, errors.New("foundry rate limited"))
	}
	if status >= http.StatusInternalServerError {
		return temporary("Foundry upstream unavailable")
	}
	return permanent("Foundry upstream rejected request")
}

func rpcStatusError(code int) error {
	if code == -32006 || code == -32007 {
		return domain.NewCollectionError(domain.CollectionErrorAuthentication, errors.New("foundry authorization consent is required"))
	}
	return parse("Foundry MCP tool failed")
}

func permanent(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorPermanent, errors.New(message))
}

func temporary(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorTemporary, errors.New(message))
}

func parse(message string) error {
	return domain.NewCollectionError(domain.CollectionErrorParse, errors.New(message))
}

func closeResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes+1))
	_ = response.Body.Close()
}
