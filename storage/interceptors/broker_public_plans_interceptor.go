/*
 * Copyright 2018 The Service Manager Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package interceptors

import (
	"context"
	"fmt"
	"time"

	"github.com/Peripli/service-manager/pkg/log"
	"github.com/Peripli/service-manager/pkg/query"
	"github.com/Peripli/service-manager/pkg/types"
	"github.com/Peripli/service-manager/storage"
	"github.com/gofrs/uuid"
)

const (
	CreateBrokerPublicPlanInterceptorName = "CreateBrokerPublicPlansInterceptor"
	UpdateBrokerPublicPlanInterceptorName = "UpdateBrokerPublicPlansInterceptor"
)

type publicPlanProcessor func(broker *types.ServiceBroker, catalogService *types.ServiceOffering, catalogPlan *types.ServicePlan) (bool, error)

type PublicPlanCreateInterceptorProvider struct {
	IsCatalogPlanPublicFunc publicPlanProcessor
	SupportedPlatforms      func(plan *types.ServicePlan) []string
}

func (p *PublicPlanCreateInterceptorProvider) Provide() storage.CreateInterceptor {
	return &publicPlanCreateInterceptor{
		isCatalogPlanPublicFunc: p.IsCatalogPlanPublicFunc,
		supportedPlatforms:      p.SupportedPlatforms,
	}
}

func (p *PublicPlanCreateInterceptorProvider) Name() string {
	return CreateBrokerPublicPlanInterceptorName
}

type PublicPlanUpdateInterceptorProvider struct {
	IsCatalogPlanPublicFunc publicPlanProcessor
	SupportedPlatforms      func(plan *types.ServicePlan) []string
}

func (p *PublicPlanUpdateInterceptorProvider) Name() string {
	return UpdateBrokerPublicPlanInterceptorName
}

func (p *PublicPlanUpdateInterceptorProvider) Provide() storage.UpdateInterceptor {
	return &publicPlanUpdateInterceptor{
		isCatalogPlanPublicFunc: p.IsCatalogPlanPublicFunc,
		supportedPlatforms:      p.SupportedPlatforms,
	}
}

type publicPlanCreateInterceptor struct {
	isCatalogPlanPublicFunc publicPlanProcessor
	supportedPlatforms      func(plan *types.ServicePlan) []string
}

func (p *publicPlanCreateInterceptor) AroundTxCreate(h storage.InterceptCreateAroundTxFunc) storage.InterceptCreateAroundTxFunc {
	return h
}

func (p *publicPlanCreateInterceptor) OnTxCreate(f storage.InterceptCreateOnTxFunc) storage.InterceptCreateOnTxFunc {
	return func(ctx context.Context, txStorage storage.Repository, obj types.Object) (types.Object, error) {
		newObject, err := f(ctx, txStorage, obj)
		if err != nil {
			return nil, err
		}
		return newObject, resync(ctx, obj.(*types.ServiceBroker), txStorage, p.isCatalogPlanPublicFunc, p.supportedPlatforms)
	}
}

type publicPlanUpdateInterceptor struct {
	isCatalogPlanPublicFunc publicPlanProcessor
	supportedPlatforms      func(plan *types.ServicePlan) []string
}

func (p *publicPlanUpdateInterceptor) AroundTxUpdate(h storage.InterceptUpdateAroundTxFunc) storage.InterceptUpdateAroundTxFunc {
	return h
}

func (p *publicPlanUpdateInterceptor) OnTxUpdate(f storage.InterceptUpdateOnTxFunc) storage.InterceptUpdateOnTxFunc {
	return func(ctx context.Context, txStorage storage.Repository, oldObj, newObj types.Object, labelChanges ...*query.LabelChange) (types.Object, error) {
		result, err := f(ctx, txStorage, oldObj, newObj, labelChanges...)
		if err != nil {
			return nil, err
		}
		return result, resync(ctx, result.(*types.ServiceBroker), txStorage, p.isCatalogPlanPublicFunc, p.supportedPlatforms)
	}
}

func resync(ctx context.Context, broker *types.ServiceBroker, txStorage storage.Repository, isCatalogPlanPublicFunc publicPlanProcessor, supportedPlatforms func(*types.ServicePlan) []string) error {
	for _, serviceOffering := range broker.Services {
		for _, servicePlan := range serviceOffering.Plans {
			planID := servicePlan.ID

			isPlanPublic, err := isCatalogPlanPublicFunc(broker, serviceOffering, servicePlan)
			if err != nil {
				return err
			}

			byServicePlanID := query.ByField(query.EqualsOperator, "service_plan_id", planID)
			planVisibilities, err := txStorage.List(ctx, types.VisibilityType, byServicePlanID)
			if err != nil {
				return err
			}

			supportedPlatformTypes := supportedPlatforms(servicePlan)
			if len(supportedPlatformTypes) == 0 { // all platforms are supported -> create single visibility with empty platform ID
				err = resyncPublicPlanVisibilities(ctx, txStorage, planVisibilities, isPlanPublic, planID, broker.ID)
			} else { // not all platforms are supported -> create single visibility for each supported platform
				err = resyncPlanVisibilitiesWithSupportedPlatforms(ctx, txStorage, planVisibilities, isPlanPublic, planID, broker.ID, supportedPlatformTypes)
			}

			if err != nil {
				return err
			}
		}
	}
	return nil
}

func resyncPublicPlanVisibilities(ctx context.Context, txStorage storage.Repository, planVisibilities types.ObjectList, isPlanPublic bool, planID, brokerID string) error {
	publicVisibilityExists := false

	for i := 0; i < planVisibilities.Len(); i++ {
		visibility := planVisibilities.ItemAt(i).(*types.Visibility)
		byVisibilityID := query.ByField(query.EqualsOperator, "id", visibility.ID)

		shouldDeleteVisibility := true
		if isPlanPublic {
			if visibility.PlatformID == "" {
				publicVisibilityExists = true
				shouldDeleteVisibility = false
			}
		} else {
			if visibility.PlatformID != "" {
				shouldDeleteVisibility = false
			}
		}

		if shouldDeleteVisibility {
			if err := txStorage.Delete(ctx, types.VisibilityType, byVisibilityID); err != nil {
				return err
			}
		}
	}

	if isPlanPublic && !publicVisibilityExists {
		if err := persistVisibility(ctx, txStorage, "", planID, brokerID); err != nil {
			return err
		}
	}

	return nil
}

func resyncPlanVisibilitiesWithSupportedPlatforms(ctx context.Context, txStorage storage.Repository, planVisibilities types.ObjectList, isPlanPublic bool, planID, brokerID string, supportedPlatformTypes []string) error {
	bySupportedPlatformTypes := query.ByField(query.InOperator, "type", supportedPlatformTypes...)
	platformList, err := txStorage.List(ctx, types.PlatformType, bySupportedPlatformTypes)
	if err != nil {
		return err
	}

	supportedPlatforms := platformList.(*types.Platforms).Platforms

	for i := 0; i < planVisibilities.Len(); i++ {
		visibility := planVisibilities.ItemAt(i).(*types.Visibility)

		shouldDeleteVisibility := true

		idx, matches := platformsAnyMatchesVisibility(supportedPlatforms, visibility)
		if isPlanPublic { // trying to match the current visibility to one of the supported platforms that should have visibilities
			if matches && len(visibility.Labels) == 0 { // visibility is present, no need to create a new one or delete this one
				supportedPlatforms = append(supportedPlatforms[:idx], supportedPlatforms[idx+1:]...)
				shouldDeleteVisibility = false
			}
		} else { // trying to match the current visibility to one of the supported platforms - if match is found and it has no labels - it's a public visibility and it has to be deleted
			if matches && len(visibility.Labels) != 0 { // visibility is present, but has labels -> visibility for paid so don't delete it
				shouldDeleteVisibility = false
			}
		}

		if shouldDeleteVisibility {
			byVisibilityID := query.ByField(query.EqualsOperator, "id", visibility.ID)
			if err := txStorage.Delete(ctx, types.VisibilityType, byVisibilityID); err != nil {
				return err
			}
		}
	}

	if isPlanPublic {
		for _, platform := range supportedPlatforms {
			if err := persistVisibility(ctx, txStorage, platform.ID, planID, brokerID); err != nil {
				return err
			}
		}
	}

	return nil
}

// platformsAnyMatchesVisibility checks whether any of the platforms matches the provided visibility
func platformsAnyMatchesVisibility(platforms []*types.Platform, visibility *types.Visibility) (int, bool) {
	for i, platform := range platforms {
		if visibility.PlatformID == platform.ID {
			return i, true
		}
	}
	return -1, false
}

func persistVisibility(ctx context.Context, txStorage storage.Repository, platformID, planID, brokerID string) error {
	UUID, err := uuid.NewV4()
	if err != nil {
		return fmt.Errorf("could not generate GUID for visibility: %s", err)
	}

	currentTime := time.Now().UTC()
	visibility := &types.Visibility{
		Base: types.Base{
			ID:        UUID.String(),
			UpdatedAt: currentTime,
			CreatedAt: currentTime,
		},
		ServicePlanID: planID,
		PlatformID:    platformID,
	}

	_, err = txStorage.Create(ctx, visibility)
	if err != nil {
		return err
	}

	log.C(ctx).Debugf("Created new public visibility for broker with id (%s), plan with id (%s) and platform with id (%s)", brokerID, planID, platformID)
	return nil
}
