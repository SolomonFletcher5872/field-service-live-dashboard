package fieldservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
)

type DeviceUpdate struct {
	DeviceID       string `json:"device_id"`
	WorkOrderID    string `json:"work_order_id"`
	DispatchStatus string `json:"dispatch_status"`
	PhotoURL       string `json:"photo_url,omitempty"`
	FollowUp       bool   `json:"technician_follow_up"`
}

type UpdateResult struct {
	DeviceID       string `json:"device_id"`
	DispatchStatus string `json:"dispatch_status"`
	Decision       string `json:"decision"`
}

type Publisher interface {
	Publish(context.Context, DeviceUpdate, string) error
	ReportStatusChange(context.Context, DeviceUpdate, string) error
}

type StatusStream struct {
	mu       sync.Mutex
	statuses map[string]string
	client   Publisher
}

func NewStatusStream(client Publisher) *StatusStream {
	return &StatusStream{statuses: make(map[string]string), client: client}
}

func (s *StatusStream) Apply(ctx context.Context, update DeviceUpdate) (UpdateResult, error) {
	if update.DeviceID == "" || update.WorkOrderID == "" || update.DispatchStatus == "" {
		return UpdateResult{}, errors.New("device_id, work_order_id, and dispatch_status are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	result := UpdateResult{
		DeviceID:       update.DeviceID,
		DispatchStatus: update.DispatchStatus,
		Decision:       "unchanged",
	}
	if s.statuses[update.DeviceID] == update.DispatchStatus {
		return result, nil
	}

	key := updateKey(update)
	if err := s.client.Publish(ctx, update, key+"-event"); err != nil {
		return UpdateResult{}, err
	}
	if err := s.client.ReportStatusChange(ctx, update, key+"-metric"); err != nil {
		return UpdateResult{}, err
	}
	s.statuses[update.DeviceID] = update.DispatchStatus
	result.Decision = "published"
	return result, nil
}

func updateKey(update DeviceUpdate) string {
	sum := sha256.Sum256([]byte(update.DeviceID + "\x00" + update.WorkOrderID + "\x00" + update.DispatchStatus))
	return hex.EncodeToString(sum[:16])
}
