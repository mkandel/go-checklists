package domain

import (
	"errors"
	"testing"
)

func TestCanCreateChecklist(t *testing.T) {
	groupID := int64(7)

	tests := []struct {
		name                 string
		tenant               Tenant
		actor                User
		isCreatorGroupMember bool
		wantErr              error
	}{
		{
			name:    "not restricted allows anyone",
			tenant:  Tenant{RestrictChecklistCreation: false},
			actor:   User{IsAdmin: false},
			wantErr: nil,
		},
		{
			name:    "restricted allows admin regardless of membership",
			tenant:  Tenant{RestrictChecklistCreation: true, CreatorGroupID: &groupID},
			actor:   User{IsAdmin: true},
			wantErr: nil,
		},
		{
			name:                 "restricted allows creator group member",
			tenant:               Tenant{RestrictChecklistCreation: true, CreatorGroupID: &groupID},
			actor:                User{IsAdmin: false},
			isCreatorGroupMember: true,
			wantErr:              nil,
		},
		{
			name:                 "restricted denies non-member",
			tenant:               Tenant{RestrictChecklistCreation: true, CreatorGroupID: &groupID},
			actor:                User{IsAdmin: false},
			isCreatorGroupMember: false,
			wantErr:              ErrChecklistCreationRestricted,
		},
		{
			name:    "restricted with no creator group denies non-admin",
			tenant:  Tenant{RestrictChecklistCreation: true, CreatorGroupID: nil},
			actor:   User{IsAdmin: false},
			wantErr: ErrChecklistCreationRestricted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanCreateChecklist(&tt.tenant, &tt.actor, tt.isCreatorGroupMember)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got %v, want %v", err, tt.wantErr)
			}
		})
	}
}
