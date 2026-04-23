//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/roman-vasich/observability-and-integration-testing/testing/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ChargeRequest struct {
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type ChargeResponse struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
}

type PaymentClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c PaymentClient) Charge(ctx context.Context, req ChargeRequest) (ChargeResponse, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ChargeResponse{}, fmt.Errorf("marshal charge request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/payments/charge", bytes.NewReader(body))
	if err != nil {
		return ChargeResponse{}, fmt.Errorf("create charge request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return ChargeResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(resp.Body)
		return ChargeResponse{}, fmt.Errorf("charge failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var out ChargeResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChargeResponse{}, fmt.Errorf("decode charge response: %w", err)
	}
	return out, nil
}

func TestPaymentClient_ExternalAPI(t *testing.T) {
	requireDocker(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	wiremock, err := setup.StartWireMock(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = wiremock.Close(context.Background())
	})

	t.Run("success", func(t *testing.T) {
		registerWireMockStub(t, wiremock.BaseURL, map[string]any{
			"request": map[string]any{
				"method": "POST",
				"url":    "/payments/charge",
				"bodyPatterns": []map[string]string{
					{"contains": `"amount_cents":1000`},
				},
			},
			"response": map[string]any{
				"status": 200,
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
				"jsonBody": map[string]any{
					"transaction_id": "txn_123",
					"status":         "approved",
				},
			},
		})

		client := PaymentClient{
			BaseURL:    wiremock.BaseURL,
			HTTPClient: &http.Client{Timeout: 2 * time.Second},
		}

		res, err := client.Charge(ctx, ChargeRequest{AmountCents: 1000, Currency: "USD"})
		require.NoError(t, err)
		assert.Equal(t, "txn_123", res.TransactionID)
		assert.Equal(t, "approved", res.Status)
	})

	t.Run("failure", func(t *testing.T) {
		registerWireMockStub(t, wiremock.BaseURL, map[string]any{
			"request": map[string]any{
				"method": "POST",
				"url":    "/payments/charge",
				"bodyPatterns": []map[string]string{
					{"contains": `"amount_cents":2000`},
				},
			},
			"response": map[string]any{
				"status": 500,
				"headers": map[string]string{
					"Content-Type": "text/plain",
				},
				"body": "internal error",
			},
		})

		client := PaymentClient{
			BaseURL:    wiremock.BaseURL,
			HTTPClient: &http.Client{Timeout: 2 * time.Second},
		}

		_, err := client.Charge(ctx, ChargeRequest{AmountCents: 2000, Currency: "USD"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status=500")
	})

	t.Run("timeout", func(t *testing.T) {
		registerWireMockStub(t, wiremock.BaseURL, map[string]any{
			"request": map[string]any{
				"method": "POST",
				"url":    "/payments/charge",
				"bodyPatterns": []map[string]string{
					{"contains": `"amount_cents":3000`},
				},
			},
			"response": map[string]any{
				"status":                 200,
				"fixedDelayMilliseconds": 1500,
				"headers": map[string]string{
					"Content-Type": "application/json",
				},
				"jsonBody": map[string]any{
					"transaction_id": "txn_timeout",
					"status":         "approved",
				},
			},
		})

		client := PaymentClient{
			BaseURL:    wiremock.BaseURL,
			HTTPClient: &http.Client{Timeout: 100 * time.Millisecond},
		}

		_, err := client.Charge(ctx, ChargeRequest{AmountCents: 3000, Currency: "USD"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Client.Timeout"), "expected timeout error, got %v", err)
	})
}

func registerWireMockStub(t *testing.T, baseURL string, mapping map[string]any) {
	t.Helper()

	payload, err := json.Marshal(mapping)
	if err != nil {
		t.Fatalf("marshal wiremock mapping: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/__admin/mappings", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("create wiremock request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	if assert.Contains(t, []int{http.StatusCreated, http.StatusOK}, resp.StatusCode) {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	t.Fatalf("unexpected wiremock status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
