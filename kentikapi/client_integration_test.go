package kentikapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/kentik/community_sdk_golang/kentikapi"
	"github.com/kentik/community_sdk_golang/kentikapi/internal/testutil"
	"github.com/kentik/community_sdk_golang/kentikapi/models"
)

func TestClient_GetUserWithRetries(t *testing.T) {
	tests := []struct {
		name                string
		responses           []testutil.HTTPResponse
		serverHandlingDelay time.Duration
		timeout             time.Duration
		expectedResult      *models.User
		expectedError       bool
		expectedTimeout     bool
		errorPredicates     []func(error) bool
	}{
		{
			name: "retry on status 502 Bad Gateway until invalid response format received",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				{StatusCode: http.StatusOK, Body: "invalid JSON"},
			},
			expectedError: true,
		}, {
			name: "retry till success when status 429 Too Many Requests received",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				{
					StatusCode: http.StatusOK,
					Body: `{
							"user": {
								"id": "145999",
								"username": "testuser",
								"user_full_name": "Test User",
								"user_email": "test@user.example",
								"role": "Member",
								"email_service": true,
								"email_product": true,
								"last_login": null,
								"created_date": "2020-12-09T14:48:42.187Z",
								"updated_date": "2020-12-09T14:48:43.243Z",
								"company_id": "74333",
								"user_api_token": "****************************a997",
								"filters": {},
								"saved_filters": []
							}
						}`,
				},
			},
			expectedResult: &models.User{
				ID:           "145999",
				Username:     "testuser",
				UserFullName: "Test User",
				UserEmail:    "test@user.example",
				Role:         "Member",
				EmailService: true,
				EmailProduct: true,
				LastLogin:    nil,
				CreatedDate:  *testutil.ParseISO8601Timestamp(t, "2020-12-09T14:48:42.187Z"),
				UpdatedDate:  *testutil.ParseISO8601Timestamp(t, "2020-12-09T14:48:43.243Z"),
				CompanyID:    "74333",
				UserAPIToken: new("****************************a997"),
			},
		}, {
			name: "default timeout is longer than 30 ms",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				{StatusCode: http.StatusBadRequest, Body: `{"error":"Bad Request"}`},
			},
			serverHandlingDelay: 10 * time.Millisecond,
			expectedError:       true,
			expectedTimeout:     false,
			errorPredicates:     []func(error) bool{kentikapi.IsInvalidRequestError},
		}, {
			name: "timeout is longer than the wait for response with retries",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				{StatusCode: http.StatusBadRequest, Body: `{"error":"Bad Request"}`},
			},
			serverHandlingDelay: 10 * time.Millisecond,
			timeout:             10 * time.Second,
			expectedError:       true,
			expectedTimeout:     false,
			errorPredicates:     []func(error) bool{kentikapi.IsInvalidRequestError},
		}, {
			name: "timeout during first request",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				{StatusCode: http.StatusBadRequest, Body: `{"error":"Bad Request"}`},
			},
			serverHandlingDelay: 10 * time.Millisecond,
			timeout:             5 * time.Millisecond,
			expectedError:       true,
			expectedTimeout:     true,
			errorPredicates:     []func(error) bool{kentikapi.IsTimeoutError, kentikapi.IsTemporaryError},
		}, {
			name: "authorization error",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusUnauthorized),
			},
			expectedError:   true,
			expectedTimeout: false,
			errorPredicates: []func(error) bool{kentikapi.IsAuthError},
		}, {
			name: "too many requests error",
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
			},
			expectedError:   true,
			expectedTimeout: false,
			errorPredicates: []func(error) bool{kentikapi.IsRateLimitExhaustedError, kentikapi.IsTemporaryError},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			h := testutil.NewMultipleResponseSpyHTTPHandler(t, tt.responses, tt.serverHandlingDelay)
			s := httptest.NewServer(h)
			defer s.Close()

			c, err := kentikapi.NewClient(
				kentikapi.WithAPIURL(s.URL),
				kentikapi.WithCredentials(dummyAuthEmail, dummyAuthToken),
				kentikapi.WithTimeout(tt.timeout),
				kentikapi.WithRetryMinDelay(1*time.Microsecond),
				kentikapi.WithRetryMaxDelay(10*time.Microsecond),
				kentikapi.WithLogPayloads(),
			)
			assert.NoError(t, err)

			// act
			result, err := c.Users.Get(context.Background(), testUserID)
			s.Close() // wait for in-flight handlers before reading the spy's state

			// assert
			t.Logf("Got result: %v, err: %v", result, err)
			if tt.expectedError {
				assert.Error(t, err)
				for _, isErr := range tt.errorPredicates {
					assert.True(t, isErr(err))
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedTimeout {
				assert.True(t, kentikapi.IsTimeoutError(err))
			} else {
				assert.False(t, kentikapi.IsTimeoutError(err))
			}

			if !tt.expectedTimeout {
				assert.Equal(t, len(tt.responses), len(h.Requests), "invalid number of requests")
			}

			for _, r := range h.Requests {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, fmt.Sprintf("/api/v5/user/%v", testUserID), r.URL.Path)
				assert.Equal(t, dummyAuthEmail, r.Header.Get(authEmailKey))
				assert.Equal(t, dummyAuthToken, r.Header.Get(authAPITokenKey))
			}

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestClient_CreateUserWithRetries(t *testing.T) {
	tests := []struct {
		name    string
		options []kentikapi.ClientOption
		user    models.User
		// expectedRequestBody is a map for the granularity of assertion diff
		expectedRequestBody map[string]any
		responses           []testutil.HTTPResponse
		expectedResult      *models.User
		expectedError       bool
	}{
		{
			name:                "retry 4 times and when status 429, 502, 503, 504 received and last status is 429",
			user:                models.User{},
			expectedRequestBody: newEmptyUserRequestBody(),
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusServiceUnavailable),
				testutil.NewErrorHTTPResponse(http.StatusGatewayTimeout),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
			},
			expectedError: true,
		}, {
			name:                "retry 5 times when status 429, 502, 503, 504 received and last status is 502",
			user:                models.User{},
			expectedRequestBody: newEmptyUserRequestBody(),
			options:             []kentikapi.ClientOption{kentikapi.WithRetryMaxAttempts(5)},
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
				testutil.NewErrorHTTPResponse(http.StatusServiceUnavailable),
				testutil.NewErrorHTTPResponse(http.StatusGatewayTimeout),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusBadGateway),
			},
			expectedError: true,
		}, {
			name: "retry till success when status 429 too many requests received",
			user: *models.NewUser(models.UserRequiredFields{
				Username:     "testuser",
				UserFullName: "Test User",
				UserEmail:    "test@user.example",
				Role:         "Member",
				EmailService: true,
				EmailProduct: false,
			}),
			expectedRequestBody: object{
				"user": object{
					"username":       "testuser",
					"user_full_name": "Test User",
					"user_email":     "test@user.example",
					"role":           "Member",
					"email_service":  true,
					"email_product":  false,
				},
			},
			responses: []testutil.HTTPResponse{
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				testutil.NewErrorHTTPResponse(http.StatusTooManyRequests),
				{
					StatusCode: http.StatusCreated,
					Body: `{
						"user": {
							"id": "145999",
							"username": "testuser",
							"user_full_name": "Test User",
							"user_email": "test@user.example",
							"role": "Member",
							"email_service": "true",
							"email_product": "false",
							"last_login": null,
							"created_date": "2020-12-09T14:48:42.187Z",
							"updated_date": "2020-12-09T14:48:43.243Z",
							"company_id": "74333",
							"user_api_token": null,
							"filters": {},
							"saved_filters": []
						}
					}`,
				},
			},
			expectedResult: &models.User{
				ID:           "145999",
				Username:     "testuser",
				UserFullName: "Test User",
				UserEmail:    "test@user.example",
				Role:         "Member",
				EmailService: true,
				EmailProduct: false,
				LastLogin:    nil,
				CreatedDate:  *testutil.ParseISO8601Timestamp(t, "2020-12-09T14:48:42.187Z"),
				UpdatedDate:  *testutil.ParseISO8601Timestamp(t, "2020-12-09T14:48:43.243Z"),
				CompanyID:    "74333",
				UserAPIToken: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// arrange
			h := testutil.NewMultipleResponseSpyHTTPHandler(t, tt.responses, 0)
			s := httptest.NewServer(h)
			defer s.Close()

			options := []kentikapi.ClientOption{
				kentikapi.WithAPIURL(s.URL),
				kentikapi.WithCredentials(dummyAuthEmail, dummyAuthToken),
				kentikapi.WithRetryMinDelay(1 * time.Microsecond),
				kentikapi.WithRetryMaxDelay(10 * time.Microsecond),
				kentikapi.WithLogPayloads(),
			}
			c, err := kentikapi.NewClient(append(options, tt.options...)...)
			assert.NoError(t, err)

			// act
			result, err := c.Users.
				Create(context.Background(), tt.user)

			// assert
			t.Logf("Got result: %v, err: %v", result, err)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, len(tt.responses), len(h.Requests), "invalid number of requests")
			for _, r := range h.Requests {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v5/user", r.URL.Path)
				assert.Equal(t, dummyAuthEmail, r.Header.Get(authEmailKey))
				assert.Equal(t, dummyAuthToken, r.Header.Get(authAPITokenKey))
				assert.Equal(t, tt.expectedRequestBody, testutil.UnmarshalJSONToIf(t, r.Body))
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
