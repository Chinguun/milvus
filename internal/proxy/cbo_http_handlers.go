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

package proxy

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	mhttp "github.com/milvus-io/milvus/internal/http"
	"github.com/milvus-io/milvus/internal/proxy/cbo"
	"github.com/milvus-io/milvus/pkg/v2/util/paramtable"
)

// getCBOMetrics returns CBO metrics for queries
// Query parameters:
//   - query_id: specific query ID to retrieve
//   - collection_id: filter by collection ID
//   - collection_name: filter by collection name
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBOMetrics(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		queryID := c.Query("query_id")
		if queryID != "" {
			metrics, exists := collector.GetMetrics(queryID)
			if !exists {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
					mhttp.HTTPReturnMessage: "Query metrics not found",
				})
				return
			}
			// Apply redaction if needed
			redactFilterExpr := shouldRedactFilterExpr(c)
			if redactFilterExpr && metrics != nil {
				redactedMetrics := *metrics
				redactedMetrics.FilterExpression = "[REDACTED]"
				c.JSON(http.StatusOK, gin.H{
					mhttp.HTTPReturnData: &redactedMetrics,
				})
			} else {
				c.JSON(http.StatusOK, gin.H{
					mhttp.HTTPReturnData: metrics,
				})
			}
			return
		}

		// Filter by collection
		var collectionID int64
		if collIDStr := c.Query("collection_id"); collIDStr != "" {
			var err error
			collectionID, err = strconv.ParseInt(collIDStr, 10, 64)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
				})
				return
			}
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			var err error
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			var err error
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		var metrics []*cbo.CBOMetrics
		if collectionID > 0 {
			metrics = collector.GetMetricsByCollection(collectionID, startTime, endTime)
		} else if !startTime.IsZero() || !endTime.IsZero() {
			metrics = collector.GetMetricsByTimeRange(startTime, endTime)
		} else {
			metrics = collector.GetAllMetrics()
		}

		// Parse pagination parameters
		limit := 100 // default
		offset := 0
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
				if parsedLimit > 1000 {
					parsedLimit = 1000 // max limit
				}
				limit = parsedLimit
			}
		}
		if offsetStr := c.Query("offset"); offsetStr != "" {
			if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		// Parse sort parameter
		sortOrder := c.DefaultQuery("sort", "timestamp_desc")
		if sortOrder == "timestamp_asc" {
			cbo.SortMetricsByTimestamp(metrics, "asc")
		} else {
			// Default to descending
			cbo.SortMetricsByTimestamp(metrics, "desc")
		}

		// Apply pagination
		paginatedMetrics, totalCount := cbo.PaginateMetrics(metrics, limit, offset)

		// Apply redaction if needed
		redactFilterExpr := shouldRedactFilterExpr(c)
		if redactFilterExpr {
			for _, m := range paginatedMetrics {
				if m != nil {
					m.FilterExpression = "[REDACTED]"
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: gin.H{
				"metrics":     paginatedMetrics,
				"count":        len(paginatedMetrics),
				"total_count": totalCount,
				"limit":       limit,
				"offset":      offset,
			},
		})
	}
}

// shouldRedactFilterExpr determines if filter expressions should be redacted
// Returns true if redaction is needed, false otherwise
func shouldRedactFilterExpr(c *gin.Context) bool {
	// Check config flag
	exposeByConfig := paramtable.Get().ProxyCfg.CBOEvaluationExposeFilterExpr.GetAsBool()
	if !exposeByConfig {
		// Config says don't expose, so redact unless explicitly requested
		includeFilterExpr := c.Query("include_filter_expr")
		return includeFilterExpr != "true"
	}
	// Config allows exposure, check query param
	includeFilterExpr := c.Query("include_filter_expr")
	return includeFilterExpr != "true"
}

// getCBOEvaluation returns CBO evaluation summary
// Query parameters:
//   - collection_id: collection ID (required)
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBOEvaluation(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		collectionIDStr := c.Query("collection_id")
		if collectionIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "collection_id parameter is required",
			})
			return
		}

		collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
			})
			return
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		summary := collector.GetCBOEvaluationSummary(ctx, collectionID, startTime, endTime)

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: summary,
		})
	}
}

// getCBOComparison returns CBO performance comparison
// Query parameters:
//   - collection_id: collection ID (required)
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBOComparison(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		collectionIDStr := c.Query("collection_id")
		if collectionIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "collection_id parameter is required",
			})
			return
		}

		collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
			})
			return
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		comparison := collector.GetCBOComparison(ctx, collectionID, startTime, endTime)

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: comparison,
		})
	}
}

// getCBORecommendations returns CBO optimization recommendations
// Query parameters:
//   - collection_id: collection ID (required)
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBORecommendations(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		collectionIDStr := c.Query("collection_id")
		if collectionIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "collection_id parameter is required",
			})
			return
		}

		collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
			})
			return
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		recommendations := collector.GetCBORecommendations(ctx, collectionID, startTime, endTime)

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: gin.H{
				"recommendations": recommendations,
				"count":           len(recommendations),
			},
		})
	}
}

// getCBOOverview returns a comprehensive overview combining summary and comparison
// Query parameters:
//   - collection_id: collection ID (required)
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBOOverview(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		collectionIDStr := c.Query("collection_id")
		if collectionIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "collection_id parameter is required",
			})
			return
		}

		collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
			})
			return
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		overview := collector.GetCBOOverview(ctx, collectionID, startTime, endTime)

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: overview,
		})
	}
}

// getCBOTop returns top-N degraded or improved queries
// Query parameters:
//   - collection_id: collection ID (required)
//   - order: "degraded" or "improved" (default: "degraded")
//   - limit: maximum number of results (default: 10, max: 100)
//   - start_time: start time for time range (RFC3339 format)
//   - end_time: end time for time range (RFC3339 format)
func getCBOTop(node *Proxy) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		collector := cbo.GetGlobalCBOMetricsCollector()

		if collector == nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				mhttp.HTTPReturnMessage: "CBO evaluation is not enabled",
			})
			return
		}

		collectionIDStr := c.Query("collection_id")
		if collectionIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "collection_id parameter is required",
			})
			return
		}

		collectionID, err := strconv.ParseInt(collectionIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid collection_id parameter",
			})
			return
		}

		// Parse order parameter
		order := c.DefaultQuery("order", "degraded")
		if order != "degraded" && order != "improved" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				mhttp.HTTPReturnMessage: "Invalid order parameter, must be 'degraded' or 'improved'",
			})
			return
		}

		// Parse limit parameter
		limit := 10 // default
		if limitStr := c.Query("limit"); limitStr != "" {
			if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
				if parsedLimit > 100 {
					parsedLimit = 100 // max limit
				}
				limit = parsedLimit
			}
		}

		// Parse time range
		var startTime, endTime time.Time
		if startTimeStr := c.Query("start_time"); startTimeStr != "" {
			startTime, err = time.Parse(time.RFC3339, startTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid start_time format, use RFC3339",
				})
				return
			}
		}
		if endTimeStr := c.Query("end_time"); endTimeStr != "" {
			endTime, err = time.Parse(time.RFC3339, endTimeStr)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					mhttp.HTTPReturnMessage: "Invalid end_time format, use RFC3339",
				})
				return
			}
		}

		topQueries := collector.GetCBOTop(ctx, collectionID, order, limit, startTime, endTime)

		// Apply redaction if needed
		redactFilterExpr := shouldRedactFilterExpr(c)
		if redactFilterExpr {
			for _, m := range topQueries {
				if m != nil {
					m.FilterExpression = "[REDACTED]"
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			mhttp.HTTPReturnData: gin.H{
				"queries": topQueries,
				"count":   len(topQueries),
				"order":   order,
			},
		})
	}
}

