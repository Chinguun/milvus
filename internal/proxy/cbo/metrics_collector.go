// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cbo

import (
	"context"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/milvus-io/milvus/pkg/v2/log"
	"github.com/milvus-io/milvus/pkg/v2/util/paramtable"
)

var (
	globalCBOMetricsCollector     *CBOMetricsCollector
	globalCBOMetricsCollectorOnce sync.Once
)

// CBOMetricsCollector collects and stores CBO-related metrics
type CBOMetricsCollector struct {
	mu       sync.RWMutex
	metrics  map[string]*CBOMetrics // key: queryID
	maxSize  int
	retention time.Duration

	// Indexes for efficient querying
	byCollection map[int64][]string // collectionID -> queryIDs
	byTimeRange  []string            // sorted queryIDs by timestamp
}

// NewCBOMetricsCollector creates a new CBO metrics collector
func NewCBOMetricsCollector(maxSize int, retention time.Duration) *CBOMetricsCollector {
	return &CBOMetricsCollector{
		metrics:      make(map[string]*CBOMetrics),
		maxSize:      maxSize,
		retention:    retention,
		byCollection: make(map[int64][]string),
		byTimeRange:  make([]string, 0),
	}
}

// RecordMetrics records CBO metrics for a query
func (c *CBOMetricsCollector) RecordMetrics(ctx context.Context, metrics *CBOMetrics) {
	if metrics == nil {
		return
	}

	// Early return if collector is disabled (maxSize <= 0)
	if c.maxSize <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Clean up old metrics if needed
	c.cleanupLocked(ctx)

	// Check if we need to evict old metrics
	if len(c.metrics) >= c.maxSize {
		c.evictOldestLocked()
	}

	// Calculate optimization result if not already calculated
	if metrics.OptimizationResult == "" {
		metrics.CalculateOptimizationResult()
	}

	// Store metrics
	c.metrics[metrics.QueryID] = metrics

	// Update indexes
	c.updateIndexesLocked(metrics)

	log.Ctx(ctx).Debug("CBO metrics recorded",
		zap.String("queryID", metrics.QueryID),
		zap.String("collection", metrics.CollectionName),
		zap.String("strategy", metrics.SelectedStrategy),
		zap.Float64("selectivity", metrics.SelectivityEstimate),
		zap.Duration("executionTime", metrics.ExecutionTimeWithCBO),
		zap.String("result", string(metrics.OptimizationResult)))
}

// GetMetrics retrieves metrics for a specific query ID
func (c *CBOMetricsCollector) GetMetrics(queryID string) (*CBOMetrics, bool) {
	// Early return if collector is disabled
	if c.maxSize <= 0 {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	metrics, exists := c.metrics[queryID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	return c.copyMetrics(metrics), true
}

// GetMetricsByCollection retrieves all metrics for a specific collection
func (c *CBOMetricsCollector) GetMetricsByCollection(collectionID int64, startTime, endTime time.Time) []*CBOMetrics {
	// Early return if collector is disabled
	if c.maxSize <= 0 {
		return []*CBOMetrics{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	queryIDs, exists := c.byCollection[collectionID]
	if !exists {
		return []*CBOMetrics{}
	}

	result := make([]*CBOMetrics, 0, len(queryIDs))
	for _, queryID := range queryIDs {
		metrics, exists := c.metrics[queryID]
		if !exists {
			continue
		}

		// Filter by time range
		if !startTime.IsZero() && metrics.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && metrics.Timestamp.After(endTime) {
			continue
		}

		result = append(result, c.copyMetrics(metrics))
	}

	return result
}

// GetMetricsByTimeRange retrieves all metrics within a time range
func (c *CBOMetricsCollector) GetMetricsByTimeRange(startTime, endTime time.Time) []*CBOMetrics {
	// Early return if collector is disabled
	if c.maxSize <= 0 {
		return []*CBOMetrics{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*CBOMetrics, 0)

	for _, queryID := range c.byTimeRange {
		metrics, exists := c.metrics[queryID]
		if !exists {
			continue
		}

		if !startTime.IsZero() && metrics.Timestamp.Before(startTime) {
			continue
		}
		if !endTime.IsZero() && metrics.Timestamp.After(endTime) {
			continue
		}

		result = append(result, c.copyMetrics(metrics))
	}

	return result
}

// GetAllMetrics retrieves all stored metrics
func (c *CBOMetricsCollector) GetAllMetrics() []*CBOMetrics {
	// Early return if collector is disabled
	if c.maxSize <= 0 {
		return []*CBOMetrics{}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*CBOMetrics, 0, len(c.metrics))
	for _, metrics := range c.metrics {
		result = append(result, c.copyMetrics(metrics))
	}

	return result
}

// SortMetricsByTimestamp sorts metrics by timestamp
// sortOrder: "asc" for ascending, "desc" for descending (default)
func SortMetricsByTimestamp(metrics []*CBOMetrics, sortOrder string) {
	if sortOrder == "asc" {
		sort.Slice(metrics, func(i, j int) bool {
			return metrics[i].Timestamp.Before(metrics[j].Timestamp)
		})
	} else {
		// Default to descending
		sort.Slice(metrics, func(i, j int) bool {
			return metrics[j].Timestamp.Before(metrics[i].Timestamp)
		})
	}
}

// PaginateMetrics applies pagination to metrics slice
// Returns the paginated slice and total count
func PaginateMetrics(metrics []*CBOMetrics, limit, offset int) ([]*CBOMetrics, int) {
	total := len(metrics)
	if offset >= total {
		return []*CBOMetrics{}, total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return metrics[offset:end], total
}

// Clear clears all metrics
func (c *CBOMetricsCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.metrics = make(map[string]*CBOMetrics)
	c.byCollection = make(map[int64][]string)
	c.byTimeRange = make([]string, 0)
}

// cleanupLocked removes expired metrics based on retention period
func (c *CBOMetricsCollector) cleanupLocked(ctx context.Context) {
	if c.retention <= 0 {
		return
	}

	now := time.Now()
	expired := make([]string, 0)

	for queryID, metrics := range c.metrics {
		if now.Sub(metrics.Timestamp) > c.retention {
			expired = append(expired, queryID)
		}
	}

	for _, queryID := range expired {
		delete(c.metrics, queryID)
	}

	if len(expired) > 0 {
		log.Ctx(ctx).Debug("CBO metrics cleanup",
			zap.Int("expired_count", len(expired)))
		c.rebuildIndexesLocked()
	}
}

// evictOldestLocked removes the oldest metrics when max size is reached
func (c *CBOMetricsCollector) evictOldestLocked() {
	if len(c.byTimeRange) == 0 {
		return
	}

	// Remove oldest entry
	oldestQueryID := c.byTimeRange[0]
	delete(c.metrics, oldestQueryID)
	c.byTimeRange = c.byTimeRange[1:]

	// Rebuild collection index
	c.rebuildIndexesLocked()
}

// updateIndexesLocked updates internal indexes for efficient querying
func (c *CBOMetricsCollector) updateIndexesLocked(metrics *CBOMetrics) {
	// Update collection index
	queryIDs := c.byCollection[metrics.CollectionID]
	found := false
	for _, id := range queryIDs {
		if id == metrics.QueryID {
			found = true
			break
		}
	}
	if !found {
		c.byCollection[metrics.CollectionID] = append(queryIDs, metrics.QueryID)
	}

	// Update time range index (keep sorted)
	// Find insertion point
	insertPos := sort.Search(len(c.byTimeRange), func(i int) bool {
		existingMetrics := c.metrics[c.byTimeRange[i]]
		return existingMetrics == nil || !existingMetrics.Timestamp.Before(metrics.Timestamp)
	})

	// Check if already exists
	for i, id := range c.byTimeRange {
		if id == metrics.QueryID {
			// Remove old position
			c.byTimeRange = append(c.byTimeRange[:i], c.byTimeRange[i+1:]...)
			if insertPos > i {
				insertPos--
			}
			break
		}
	}

	// Insert at correct position
	if insertPos >= len(c.byTimeRange) {
		c.byTimeRange = append(c.byTimeRange, metrics.QueryID)
	} else {
		c.byTimeRange = append(c.byTimeRange[:insertPos], append([]string{metrics.QueryID}, c.byTimeRange[insertPos:]...)...)
	}
}

// rebuildIndexesLocked rebuilds all indexes from scratch
func (c *CBOMetricsCollector) rebuildIndexesLocked() {
	c.byCollection = make(map[int64][]string)
	c.byTimeRange = make([]string, 0, len(c.metrics))

	// Collect all metrics and sort by timestamp
	type metricWithID struct {
		queryID string
		metrics *CBOMetrics
	}
	allMetrics := make([]metricWithID, 0, len(c.metrics))
	for queryID, metrics := range c.metrics {
		allMetrics = append(allMetrics, metricWithID{queryID: queryID, metrics: metrics})
	}

	// Sort by timestamp
	sort.Slice(allMetrics, func(i, j int) bool {
		return allMetrics[i].metrics.Timestamp.Before(allMetrics[j].metrics.Timestamp)
	})

	// Rebuild indexes
	for _, m := range allMetrics {
		c.byTimeRange = append(c.byTimeRange, m.queryID)
		c.byCollection[m.metrics.CollectionID] = append(c.byCollection[m.metrics.CollectionID], m.queryID)
	}
}

// copyMetrics creates a deep copy of metrics to avoid race conditions
func (c *CBOMetricsCollector) copyMetrics(metrics *CBOMetrics) *CBOMetrics {
	if metrics == nil {
		return nil
	}

	return &CBOMetrics{
		QueryID:              metrics.QueryID,
		CollectionID:         metrics.CollectionID,
		CollectionName:       metrics.CollectionName,
		FilterExpression:     metrics.FilterExpression,
		Timestamp:            metrics.Timestamp,
		SelectivityEstimate:  metrics.SelectivityEstimate,
		SelectedStrategy:     metrics.SelectedStrategy,
		EstimatorType:        metrics.EstimatorType,
		DecisionApplied:      metrics.DecisionApplied,
		DecisionSkipReason:   metrics.DecisionSkipReason,
		SelectivityThreshold: metrics.SelectivityThreshold,
		HintsBefore:          metrics.HintsBefore,
		HintsAfter:           metrics.HintsAfter,
		BaselineSampled:      metrics.BaselineSampled,
		BaselineAttempted:    metrics.BaselineAttempted,
		BaselineStatus:       metrics.BaselineStatus,
		BaselineErrorCode:    metrics.BaselineErrorCode,
		BaselineErrorMsg:     metrics.BaselineErrorMsg,
		ExecutionTimeWithCBO: metrics.ExecutionTimeWithCBO,
		ExecutionTimeWithoutCBO: metrics.ExecutionTimeWithoutCBO,
		TimeReduction:        metrics.TimeReduction,
		TimeReductionPercent: metrics.TimeReductionPercent,
		NumQueries:           metrics.NumQueries,
		TopK:                 metrics.TopK,
		NumResults:           metrics.NumResults,
		OptimizationResult:   metrics.OptimizationResult,
		ImprovementPercent:   metrics.ImprovementPercent,
	}
}

// GetStats returns basic statistics about stored metrics
func (c *CBOMetricsCollector) GetStats() map[string]interface{} {
	// Early return if collector is disabled
	if c.maxSize <= 0 {
		return map[string]interface{}{
			"total_metrics": 0,
			"max_size":      0,
			"retention":     c.retention.String(),
			"collections":   0,
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"total_metrics":    len(c.metrics),
		"max_size":         c.maxSize,
		"retention":        c.retention.String(),
		"collections":      len(c.byCollection),
	}
}

// GetGlobalCBOMetricsCollector returns the global CBO metrics collector instance
func GetGlobalCBOMetricsCollector() *CBOMetricsCollector {
	globalCBOMetricsCollectorOnce.Do(func() {
		enabled := paramtable.Get().ProxyCfg.CBOEvaluationEnabled.GetAsBool()
		if !enabled {
			// Return a disabled collector that does nothing
			globalCBOMetricsCollector = NewCBOMetricsCollector(0, 0)
			return
		}

		maxSize := int(paramtable.Get().ProxyCfg.CBOEvaluationMaxMetrics.GetAsInt64())
		retention := paramtable.Get().ProxyCfg.CBOEvaluationRetention.GetAsDurationByParse()
		globalCBOMetricsCollector = NewCBOMetricsCollector(maxSize, retention)
	})
	return globalCBOMetricsCollector
}

