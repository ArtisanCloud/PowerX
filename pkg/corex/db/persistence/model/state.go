package model

import (
	"fmt"
	"sort"
)

// 媒体资产业务状态枚举，与 HTTP/gRPC 契约保持一致。
const (
	MediaAssetStatusDraft       = "draft"
	MediaAssetStatusUnderReview = "under_review"
	MediaAssetStatusPublished   = "published"
	MediaAssetStatusArchived    = "archived"
)

var mediaAssetStatusSet = map[string]struct{}{
	MediaAssetStatusDraft:       {},
	MediaAssetStatusUnderReview: {},
	MediaAssetStatusPublished:   {},
	MediaAssetStatusArchived:    {},
}

var mediaAssetTransitions = map[string][]string{
	MediaAssetStatusDraft: {
		MediaAssetStatusUnderReview,
		MediaAssetStatusArchived,
	},
	MediaAssetStatusUnderReview: {
		MediaAssetStatusDraft,
		MediaAssetStatusPublished,
		MediaAssetStatusArchived,
	},
	MediaAssetStatusPublished: {
		MediaAssetStatusArchived,
	},
	MediaAssetStatusArchived: {
		MediaAssetStatusDraft,
	},
}

// MediaAssetStatuses 返回所有合法业务状态（排序后复制一份）。
func MediaAssetStatuses() []string {
	statuses := make([]string, 0, len(mediaAssetStatusSet))
	for status := range mediaAssetStatusSet {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	return statuses
}

// IsValidMediaAssetStatus 判断状态值是否在受支持的枚举内。
func IsValidMediaAssetStatus(status string) bool {
	_, ok := mediaAssetStatusSet[status]
	return ok
}

// ValidateMediaAssetStatus 校验状态是否合法。
func ValidateMediaAssetStatus(status string) error {
	if !IsValidMediaAssetStatus(status) {
		return fmt.Errorf("非法的媒体资产业务状态: %s", status)
	}
	return nil
}

// AllowedMediaAssetTransitions 返回指定状态可流转到的目标状态列表。
func AllowedMediaAssetTransitions(from string) []string {
	targets, ok := mediaAssetTransitions[from]
	if !ok {
		return nil
	}
	// 返回拷贝，避免外部修改内部切片
	cloned := make([]string, len(targets))
	copy(cloned, targets)
	sort.Strings(cloned)
	return cloned
}

// CanTransitionMediaAsset 判断业务状态流转是否合法。
func CanTransitionMediaAsset(from, to string) bool {
	if from == to {
		return true
	}
	targets, ok := mediaAssetTransitions[from]
	if !ok {
		return false
	}
	for _, target := range targets {
		if target == to {
			return true
		}
	}
	return false
}

// ValidateMediaAssetTransition 校验业务状态流转是否合法。
func ValidateMediaAssetTransition(from, to string) error {
	if err := ValidateMediaAssetStatus(from); err != nil {
		return err
	}
	if err := ValidateMediaAssetStatus(to); err != nil {
		return err
	}
	if !CanTransitionMediaAsset(from, to) {
		return fmt.Errorf("媒体资产状态不允许从 %s 流转到 %s", from, to)
	}
	return nil
}
