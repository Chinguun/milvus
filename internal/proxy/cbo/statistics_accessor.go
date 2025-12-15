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
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/milvus-io/milvus-proto/go-api/v2/commonpb"
	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"github.com/milvus-io/milvus/internal/storage"
	"github.com/milvus-io/milvus/internal/types"
	"github.com/milvus-io/milvus/pkg/v2/log"
	"github.com/milvus-io/milvus/pkg/v2/proto/datapb"
	"github.com/milvus-io/milvus/pkg/v2/util/paramtable"
)

// FieldStatistics contains statistics for a single field
type FieldStatistics struct {
	FieldID     int64
	Min         interface{} // Minimum value (type depends on field data type)
	Max         interface{} // Maximum value (type depends on field data type)
	Cardinality int64      // Estimated number of distinct values
	RowCount    int64      // Total number of rows
}

// StatisticsAccessor is the interface for accessing field statistics
type StatisticsAccessor interface {
	// GetFieldStatistics retrieves statistics for a specific field
	// Returns nil if statistics are not available
	GetFieldStatistics(ctx context.Context, collectionID int64, fieldID int64) (*FieldStatistics, error)
}

// CollectionStatisticsAccessor implements StatisticsAccessor with caching
type CollectionStatisticsAccessor struct {
	cache      map[string]*FieldStatistics // Key: "collectionID:fieldID"
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
	lastAccess map[string]time.Time
	mixCoord   types.MixCoordClient // DataCoord client for fetching segment info
}

// NewCollectionStatisticsAccessor creates a new statistics accessor (without DataCoord, returns nil stats)
func NewCollectionStatisticsAccessor(cacheTTL time.Duration) *CollectionStatisticsAccessor {
	return &CollectionStatisticsAccessor{
		cache:      make(map[string]*FieldStatistics),
		cacheTTL:   cacheTTL,
		lastAccess: make(map[string]time.Time),
		mixCoord:   nil,
	}
}

// NewCollectionStatisticsAccessorWithDataCoord creates a new statistics accessor with DataCoord client
func NewCollectionStatisticsAccessorWithDataCoord(cacheTTL time.Duration, mixCoord types.MixCoordClient) *CollectionStatisticsAccessor {
	return &CollectionStatisticsAccessor{
		cache:      make(map[string]*FieldStatistics),
		cacheTTL:   cacheTTL,
		lastAccess: make(map[string]time.Time),
		mixCoord:   mixCoord,
	}
}

// GetFieldStatistics retrieves field statistics with caching
// Fetches statistics from DataCoord and storage if mixCoord is available
func (a *CollectionStatisticsAccessor) GetFieldStatistics(ctx context.Context, collectionID int64, fieldID int64) (*FieldStatistics, error) {
	key := a.cacheKey(collectionID, fieldID)

	// Check cache
	a.cacheMu.RLock()
	cached, exists := a.cache[key]
	lastAccessTime, timeExists := a.lastAccess[key]
	a.cacheMu.RUnlock()

	if exists && timeExists {
		// Check if cache is still valid
		if time.Since(lastAccessTime) < a.cacheTTL {
			return cached, nil
		}
		// Cache expired, remove it
		a.cacheMu.Lock()
		delete(a.cache, key)
		delete(a.lastAccess, key)
		a.cacheMu.Unlock()
	}

	// If no DataCoord client, return nil (fallback to mock estimator)
	if a.mixCoord == nil {
		log.Ctx(ctx).Debug("Field statistics not available (no DataCoord client), will use fallback",
			zap.Int64("collectionID", collectionID),
			zap.Int64("fieldID", fieldID))
		return nil, nil
	}

	// Fetch statistics from DataCoord and storage
	stats, err := a.fetchFieldStatisticsFromDataCoord(ctx, collectionID, fieldID)
	if err != nil {
		log.Ctx(ctx).Debug("Failed to fetch field statistics from DataCoord, will use fallback",
			zap.Int64("collectionID", collectionID),
			zap.Int64("fieldID", fieldID),
			zap.Error(err))
		return nil, nil
	}

	if stats != nil {
		// Cache the result
		a.cacheMu.Lock()
		a.cache[key] = stats
		a.lastAccess[key] = time.Now()
		a.cacheMu.Unlock()
		return stats, nil
	}

	return nil, nil
}

// fetchFieldStatisticsFromDataCoord fetches field statistics from DataCoord segments
func (a *CollectionStatisticsAccessor) fetchFieldStatisticsFromDataCoord(ctx context.Context, collectionID int64, fieldID int64) (*FieldStatistics, error) {
	// Get sealed/flushed segments for the collection
	getSegmentsResp, err := a.mixCoord.GetSegmentsByStates(ctx, &datapb.GetSegmentsByStatesRequest{
		CollectionID: collectionID,
		PartitionID:  -1, // All partitions
		States:       []commonpb.SegmentState{commonpb.SegmentState_Flushed, commonpb.SegmentState_Sealed},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get segments by states: %w", err)
	}

	if len(getSegmentsResp.Segments) == 0 {
		log.Ctx(ctx).Debug("No sealed/flushed segments found for collection",
			zap.Int64("collectionID", collectionID))
		return nil, nil
	}

	// Get segment info including stats logs
	segmentInfoResp, err := a.mixCoord.GetSegmentInfo(ctx, &datapb.GetSegmentInfoRequest{
		SegmentIDs: getSegmentsResp.Segments,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get segment info: %w", err)
	}

	if segmentInfoResp.GetStatus().GetErrorCode() != commonpb.ErrorCode_Success {
		return nil, fmt.Errorf("get segment info failed: %s", segmentInfoResp.GetStatus().GetReason())
	}

	// Aggregate statistics across segments
	var globalMin interface{}
	var globalMax interface{}
	var totalRowCount int64
	var foundStats bool

	// Create chunk manager for reading stats blobs
	chunkManagerFactory := storage.NewChunkManagerFactoryWithParam(paramtable.Get())
	chunkManager, err := chunkManagerFactory.NewPersistentStorageChunkManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunk manager: %w", err)
	}
	// Note: ChunkManager doesn't require explicit cleanup/close

	// Process each segment
	for _, segInfo := range segmentInfoResp.Infos {
		// Find stats logs for this field
		for _, fieldBinlog := range segInfo.GetStatslogs() {
			if fieldBinlog.GetFieldID() == fieldID && len(fieldBinlog.GetBinlogs()) > 0 {
				// Use the latest stats log (last in the list)
				latestBinlog := fieldBinlog.GetBinlogs()[len(fieldBinlog.GetBinlogs())-1]
				statsPath := latestBinlog.GetLogPath()

				// Read stats blob
				statsBlob, err := chunkManager.Read(ctx, statsPath)
				if err != nil {
					log.Ctx(ctx).Debug("Failed to read stats blob",
						zap.String("path", statsPath),
						zap.Error(err))
					continue
				}

				// Parse field stats
				blob := &storage.Blob{Value: statsBlob}
				fieldStatsList, err := storage.DeserializeFieldStats(blob)
				if err != nil {
					log.Ctx(ctx).Debug("Failed to deserialize field stats",
						zap.String("path", statsPath),
						zap.Error(err))
					continue
				}

				// Find stats for this field
				for _, fs := range fieldStatsList {
					if fs.FieldID == fieldID {
						foundStats = true
						totalRowCount += segInfo.GetNumOfRows()

						// Update global min/max
						if fs.Min != nil && fs.Max != nil {
							// Extract actual values from ScalarFieldValue
							minVal := fs.Min.GetValue()
							maxVal := fs.Max.GetValue()
							
							if globalMin == nil {
								globalMin = minVal
								globalMax = maxVal
							} else {
								// Compare and update min/max
								globalMin, globalMax = updateMinMax(globalMin, globalMax, minVal, maxVal)
							}
						}
						break
					}
				}
			}
		}
	}

	if !foundStats {
		return nil, nil
	}

	// Estimate cardinality (rough estimate: assume uniform distribution)
	// This is a simplified estimate - in production, could use histogram or other methods
	cardinality := totalRowCount / 10 // Rough estimate: 10% distinct values
	if cardinality <= 0 {
		cardinality = 100 // Default fallback
	}

	return &FieldStatistics{
		FieldID:     fieldID,
		Min:         globalMin,
		Max:         globalMax,
		Cardinality: cardinality,
		RowCount:    totalRowCount,
	}, nil
}

// updateMinMax updates global min/max values based on new field stats
func updateMinMax(globalMin, globalMax, newMin, newMax interface{}) (interface{}, interface{}) {
	// Handle different numeric types
	switch v1 := globalMin.(type) {
	case int8:
		if v2, ok := newMin.(int8); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(int8); ok {
			if v3, ok3 := globalMax.(int8); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	case int16:
		if v2, ok := newMin.(int16); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(int16); ok {
			if v3, ok3 := globalMax.(int16); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	case int32:
		if v2, ok := newMin.(int32); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(int32); ok {
			if v3, ok3 := globalMax.(int32); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	case int64:
		if v2, ok := newMin.(int64); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(int64); ok {
			if v3, ok3 := globalMax.(int64); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	case float32:
		if v2, ok := newMin.(float32); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(float32); ok {
			if v3, ok3 := globalMax.(float32); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	case float64:
		if v2, ok := newMin.(float64); ok && v2 < v1 {
			globalMin = v2
		}
		if v2, ok := newMax.(float64); ok {
			if v3, ok3 := globalMax.(float64); ok3 && v2 > v3 {
				globalMax = v2
			}
		}
	// For other types (string, etc.), keep first encountered values
	}
	return globalMin, globalMax
}

// SetFieldStatistics allows setting statistics manually (for testing or future use)
func (a *CollectionStatisticsAccessor) SetFieldStatistics(collectionID int64, fieldID int64, stats *FieldStatistics) {
	key := a.cacheKey(collectionID, fieldID)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.cache[key] = stats
	a.lastAccess[key] = time.Now()
}

// ClearCache clears the statistics cache
func (a *CollectionStatisticsAccessor) ClearCache() {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.cache = make(map[string]*FieldStatistics)
	a.lastAccess = make(map[string]time.Time)
}

func (a *CollectionStatisticsAccessor) cacheKey(collectionID int64, fieldID int64) string {
	return fmt.Sprintf("%d:%d", collectionID, fieldID)
}

// GetFieldDataType returns the data type of a field from schema
func GetFieldDataType(schema *schemapb.CollectionSchema, fieldID int64) schemapb.DataType {
	if schema == nil {
		return schemapb.DataType_None
	}

	for _, field := range schema.Fields {
		if field.FieldID == fieldID {
			return field.DataType
		}
	}

	return schemapb.DataType_None
}

