/*
 *    Copyright 2018 The Service Manager Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package postgres

import (
	"database/sql"
	"time"

	"github.com/Peripli/service-manager/storage"

	"github.com/Peripli/service-manager/pkg/types"
)

//go:generate smgen storage platform github.com/Peripli/service-manager/pkg/types
// Platform entity
type Platform struct {
	BaseEntity
	Type        string         `db:"type"`
	Name        string         `db:"name"`
	Description sql.NullString `db:"description"`
	Username    string         `db:"username"`
	Password    string         `db:"password"`
	Checksum    []byte         `db:"checksum"`
	Active      bool           `db:"active"`
	LastActive  time.Time      `db:"last_active"`
}

func (p *Platform) FromObject(object types.Object) (storage.Entity, bool) {
	platform, ok := object.(*types.Platform)
	if !ok {
		return nil, false
	}
	result := &Platform{
		BaseEntity: BaseEntity{
			ID:             platform.ID,
			CreatedAt:      platform.CreatedAt,
			UpdatedAt:      platform.UpdatedAt,
			PagingSequence: platform.PagingSequence,
			Ready:          platform.Ready,
		},
		Type:        platform.Type,
		Name:        platform.Name,
		Description: toNullString(platform.Description),
		Active:      platform.Active,
		LastActive:  platform.LastActive,
	}

	if platform.Description != "" {
		result.Description.Valid = true
	}
	if platform.Credentials != nil && platform.Credentials.Basic != nil {
		result.Username = platform.Credentials.Basic.Username
		result.Password = platform.Credentials.Basic.Password
		result.Checksum = platform.Credentials.Checksum[:]
	}
	return result, true
}

func (p *Platform) ToObject() types.Object {
	var checksum [32]byte
	copy(checksum[:], p.Checksum)
	return &types.Platform{
		Base: types.Base{
			ID:             p.ID,
			CreatedAt:      p.CreatedAt,
			UpdatedAt:      p.UpdatedAt,
			PagingSequence: p.PagingSequence,
			Ready:          p.Ready,
		},
		Type:        p.Type,
		Name:        p.Name,
		Description: p.Description.String,
		Credentials: &types.Credentials{
			Basic: &types.Basic{
				Username: p.Username,
				Password: p.Password,
			},
			Checksum: checksum,
		},
		Active:     p.Active,
		LastActive: p.LastActive,
	}
}
