package provider

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/http"

	intelligencedomain "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/domain"
)

type providerStatusError struct{ statusCode int }

func (err *providerStatusError) Error() string { return "AI provider returned an unsuccessful status" }

type safeStatusTransport struct{ base http.RoundTripper }

func (transport safeStatusTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusBadRequest {
		return response, nil
	}
	_, _ = io.CopyN(io.Discard, response.Body, 64<<10)
	_ = response.Body.Close()
	return nil, &providerStatusError{statusCode: response.StatusCode}
}

type safeStatusDoer struct{ client *http.Client }

func (doer safeStatusDoer) Do(request *http.Request) (*http.Response, error) {
	response, err := doer.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusBadRequest {
		return response, nil
	}
	_, _ = io.CopyN(io.Discard, response.Body, 64<<10)
	_ = response.Body.Close()
	return nil, &providerStatusError{statusCode: response.StatusCode}
}

func safeLangChainDoer(client *http.Client) safeStatusDoer {
	if client == nil {
		client = &http.Client{}
	}
	return safeStatusDoer{client: client}
}

func safeLangChainHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copyOfClient := *client
	copyOfClient.Transport = safeStatusTransport{base: base}
	return &copyOfClient
}

func mapLangChainError(err error) error {
	if err == nil {
		return nil
	}
	if stdErrors.Is(err, context.DeadlineExceeded) || stdErrors.Is(err, context.Canceled) {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}
	var networkError net.Error
	if stdErrors.As(err, &networkError) && networkError.Timeout() {
		return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
	}
	var statusError *providerStatusError
	if stdErrors.As(err, &statusError) {
		switch {
		case statusError.statusCode == http.StatusRequestTimeout:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTimeout)
		case statusError.statusCode == http.StatusTooManyRequests:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderRateLimited)
		case statusError.statusCode >= http.StatusInternalServerError:
			return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
		default:
			return intelligencedomain.NewError(intelligencedomain.CodeAIModelProfileInvalid)
		}
	}
	return intelligencedomain.NewError(intelligencedomain.CodeAIProviderTransient)
}
