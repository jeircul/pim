package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ActivateRole submits an activation or extension request.
func (c *Client) ActivateRole(ctx context.Context, role Role, principalID, justification string, minutes int, targetScope string) (*ScheduleResponse, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	minutes = ClampMinutes(minutes)

	scopePath := NormalizeScope(role.Scope)
	if strings.TrimSpace(targetScope) != "" {
		scopePath = NormalizeScope(targetScope)
	}

	active, err := c.isRoleActiveAt(ctx, scopePath, role.RoleDefinitionID, principalID)
	if err != nil {
		return nil, err
	}
	requestType := "SelfActivate"
	if active {
		requestType = "SelfExtend"
	}

	req := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:                     principalID,
			RoleDefinitionID:                role.RoleDefinitionID,
			RequestType:                     requestType,
			Justification:                   justification,
			LinkedRoleEligibilityScheduleID: role.EligibilityScheduleID,
			ScheduleInfo: &ScheduleInfo{
				StartDateTime: time.Now().UTC().Format(time.RFC3339),
				Expiration: Expiration{
					Type:     "AfterDuration",
					Duration: FormatDuration(minutes),
				},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		armEndpoint, scopePath, requestID, apiVersion)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, tok, bytes.NewReader(body))
	if err != nil {
		var apiErr *APIError
		if IsResourceGroupScope(scopePath) && errors.As(err, &apiErr) &&
			(apiErr.StatusCode == 403 || strings.EqualFold(apiErr.Code, "AuthorizationFailed")) {
			return c.activateAtSubscriptionScope(ctx, req, scopePath, err)
		}
		return nil, fmt.Errorf("submit activation: %w", err)
	}
	defer resp.Body.Close()

	return decodeScheduleResponse(resp.Body, "decode response")
}

// activateAtSubscriptionScope retries an activation request at subscription scope
// when the RG-scope PUT fails with 403. Azure requires resourceGroups/read at the
// target RG before it will process PIM requests there — a chicken-and-egg that makes
// RG-scope activation impossible without a pre-existing assignment.
func (c *Client) activateAtSubscriptionScope(ctx context.Context, req ScheduleRequest, rgScope string, rgErr error) (*ScheduleResponse, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	subID := SubscriptionIDFromScope(rgScope)
	if subID == "" {
		return nil, fmt.Errorf("submit activation: cannot determine subscription from RG scope %s", rgScope)
	}
	subScope := "/subscriptions/" + subID

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal fallback request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		armEndpoint, subScope, requestID, apiVersion)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, tok, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit activation at %s (and fallback to subscription %s): %w", rgScope, subScope, errors.Join(rgErr, err))
	}
	defer resp.Body.Close()

	return decodeScheduleResponse(resp.Body, "decode fallback response")
}

// DeactivateRole submits a role deactivation request.
func (c *Client) DeactivateRole(ctx context.Context, assignment ActiveAssignment, principalID string) (*ScheduleResponse, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}

	req := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:      principalID,
			RoleDefinitionID: assignment.RoleDefinitionID,
			RequestType:      "SelfDeactivate",
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		armEndpoint, assignment.Scope, requestID, apiVersion)

	resp, err := c.doRequest(ctx, http.MethodPut, reqURL, tok, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit deactivation: %w", err)
	}
	defer resp.Body.Close()

	return decodeScheduleResponse(resp.Body, "decode response")
}

func decodeScheduleResponse(r io.Reader, context string) (*ScheduleResponse, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", context, err)
	}
	if len(body) == 0 {
		return &ScheduleResponse{}, nil
	}
	var schedResp ScheduleResponse
	if err := json.Unmarshal(body, &schedResp); err != nil {
		return nil, fmt.Errorf("%s: %w", context, err)
	}
	return &schedResp, nil
}
