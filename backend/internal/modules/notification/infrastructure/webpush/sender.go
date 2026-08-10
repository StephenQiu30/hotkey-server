package webpush

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/StephenQiu30/hotkey-server/backend/internal/modules/notification/application"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	sharedrepository "github.com/StephenQiu30/hotkey-server/backend/internal/shared/repository"
)

type Sender struct {
	publicKey  string
	privateKey string
	subject    string
	client     *stdhttp.Client
}

var _ application.WebPushSender = (*Sender)(nil)

func NewSender(configuration config.NotificationConfig) (*Sender, error) {
	if err := configuration.ValidateWebPushRuntime(); err != nil {
		return nil, err
	}
	sender := &Sender{
		publicKey:  strings.TrimSpace(configuration.VAPIDPublicKey),
		privateKey: strings.TrimSpace(configuration.VAPIDPrivateKey),
		subject:    strings.TrimSpace(configuration.VAPIDSubject),
	}
	sender.client = &stdhttp.Client{
		Timeout: 20 * time.Second,
		Transport: &stdhttp.Transport{
			Proxy: nil, ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			DialContext:     dialPublicPushEndpoint,
		},
		CheckRedirect: func(*stdhttp.Request, []*stdhttp.Request) error {
			return fmt.Errorf("Web Push redirects are not allowed")
		},
	}
	return sender, nil
}

func (sender *Sender) SendWebPush(ctx context.Context, message application.WebPushMessageDTO) (application.WebPushSendResult, error) {
	if sender == nil || sender.publicKey == "" || sender.privateKey == "" || sender.subject == "" || sender.client == nil {
		return application.WebPushSendResult{}, sharedrepository.ErrUnavailable
	}
	if len(message.Payload) == 0 || len(message.Payload) > 3072 || message.TTL <= 0 || message.TTL > 86400 ||
		message.Endpoint == "" || message.P256DH == "" || message.Auth == "" || message.Topic == "" || len(message.Topic) > 32 {
		return application.WebPushSendResult{}, sharedrepository.ErrInvalidInput
	}
	response, err := webpush.SendNotificationWithContext(ctx, message.Payload, &webpush.Subscription{
		Endpoint: message.Endpoint,
		Keys:     webpush.Keys{P256dh: message.P256DH, Auth: message.Auth},
	}, &webpush.Options{
		HTTPClient: sender.client, Subscriber: sender.subject, Topic: message.Topic, TTL: message.TTL,
		Urgency: webpush.UrgencyNormal, VAPIDPublicKey: sender.publicKey, VAPIDPrivateKey: sender.privateKey,
	})
	if response == nil {
		return application.WebPushSendResult{}, err
	}
	defer response.Body.Close()
	return application.WebPushSendResult{StatusCode: response.StatusCode}, err
}

func dialPublicPushEndpoint(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return nil, fmt.Errorf("invalid Web Push address")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve Web Push endpoint")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for _, candidate := range addresses {
		if !publicPushIP(candidate.IP) {
			continue
		}
		connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if err == nil {
			return connection, nil
		}
	}
	return nil, fmt.Errorf("Web Push endpoint has no reachable public address")
}

func publicPushIP(address net.IP) bool {
	if address == nil || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast() {
		return false
	}
	// IPv4 documentation, benchmarking, carrier-grade NAT and reserved ranges
	// are global-unicast according to net.IP but are never valid push services.
	if ipv4 := address.To4(); ipv4 != nil {
		value := uint32(ipv4[0])<<24 | uint32(ipv4[1])<<16 | uint32(ipv4[2])<<8 | uint32(ipv4[3])
		for _, block := range [][2]uint32{
			{0x64400000, 0x647fffff}, {0xc0000000, 0xc00000ff}, {0xc0000200, 0xc00002ff},
			{0xc6120000, 0xc613ffff}, {0xc6336400, 0xc63364ff}, {0xcb007100, 0xcb0071ff},
			{0xe9fc0000, 0xe9fc00ff}, {0xf0000000, 0xffffffff},
		} {
			if value >= block[0] && value <= block[1] {
				return false
			}
		}
	}
	return true
}
