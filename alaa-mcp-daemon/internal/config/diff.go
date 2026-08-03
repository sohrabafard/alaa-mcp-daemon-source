package config

import "fmt"

type ChangeKind string

const (
	ChangeAdd            ChangeKind = "add"
	ChangeRemove         ChangeKind = "remove"
	ChangeProcessRestart ChangeKind = "process_restart"
	ChangeSupervision    ChangeKind = "supervision"
	ChangeMetadata       ChangeKind = "metadata"
	ChangeNone           ChangeKind = "none"
)

type Change struct {
	ID   string
	Kind ChangeKind
	Old  *EffectiveService
	New  *EffectiveService
}

func Diff(old, next *Effective) ([]Change, error) {
	if old == nil {
		changes := make([]Change, 0, len(next.Order))
		for _, id := range next.Order {
			s := next.Services[id]
			sCopy := s
			changes = append(changes, Change{ID: id, Kind: ChangeAdd, New: &sCopy})
		}
		return changes, nil
	}
	if old.Runtime.LogDir != next.Runtime.LogDir || old.Runtime.StateDir != next.Runtime.StateDir {
		return nil, fmt.Errorf("runtime.log_dir and runtime.state_dir cannot change during reload; restart the daemon")
	}
	changes := make([]Change, 0, len(old.Services)+len(next.Services))
	seen := make(map[string]struct{}, len(old.Services))
	for _, id := range old.Order {
		oldService := old.Services[id]
		oldCopy := oldService
		newService, exists := next.Services[id]
		if !exists {
			changes = append(changes, Change{ID: id, Kind: ChangeRemove, Old: &oldCopy})
			continue
		}
		seen[id] = struct{}{}
		newCopy := newService
		kind := ChangeNone
		switch {
		case oldService.ProcessHash != newService.ProcessHash:
			kind = ChangeProcessRestart
		case oldService.SupervisionHash != newService.SupervisionHash:
			kind = ChangeSupervision
		case oldService.MetadataHash != newService.MetadataHash:
			kind = ChangeMetadata
		}
		changes = append(changes, Change{ID: id, Kind: kind, Old: &oldCopy, New: &newCopy})
	}
	for _, id := range next.Order {
		if _, exists := seen[id]; exists {
			continue
		}
		service := next.Services[id]
		copy := service
		changes = append(changes, Change{ID: id, Kind: ChangeAdd, New: &copy})
	}
	return changes, nil
}
