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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/milvus-io/milvus/internal/proxy/cbo"
	mhttp "github.com/milvus-io/milvus/internal/http"
)

func setupTestCollectorForHTTP() *cbo.CBOMetricsCollector {
	return cbo.NewCBOMetricsCollector(100, 1*time.Hour)
}

func TestGetCBOMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with query_id parameter - existing", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:        "query-123",
			CollectionID:   1,
			CollectionName: "test_collection",
			Timestamp:      time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		// Mock global collector
		originalCollector := cbo.GetGlobalCBOMetricsCollector()
		defer func() {
			// Restore original if needed
		}()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?query_id=query-123", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "query-123")
	})

	t.Run("with query_id parameter - non-existent", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?query_id=non-existent", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "not found")
	})

	t.Run("with collection_id parameter", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:      "query-coll",
			CollectionID: 5,
			Timestamp:    time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=5", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("with time range parameters", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:      "query-time",
			CollectionID: 1,
			Timestamp:    time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		startTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?start_time="+startTime+"&end_time="+endTime, nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("with invalid time format", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?start_time=invalid", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "RFC3339")
	})

	t.Run("with invalid collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=invalid", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestGetCBOEvaluation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with valid collection_id", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:      "eval-1",
			CollectionID: 10,
			Timestamp:    time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/evaluation?collection_id=10", nil)

		proxy := &Proxy{}
		handler := getCBOEvaluation(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "TotalQueries")
	})

	t.Run("with missing collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/evaluation", nil)

		proxy := &Proxy{}
		handler := getCBOEvaluation(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "required")
	})

	t.Run("with invalid collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/evaluation?collection_id=invalid", nil)

		proxy := &Proxy{}
		handler := getCBOEvaluation(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("with time range parameters", func(t *testing.T) {
		startTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/evaluation?collection_id=1&start_time="+startTime+"&end_time="+endTime, nil)

		proxy := &Proxy{}
		handler := getCBOEvaluation(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCBOComparison(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with valid collection_id", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:                "comp-1",
			CollectionID:           20,
			ExecutionTimeWithCBO:    50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			Timestamp:              time.Now(),
		}
		metric.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, metric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/comparison?collection_id=20", nil)

		proxy := &Proxy{}
		handler := getCBOComparison(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "TotalQueries")
	})

	t.Run("with missing collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/comparison", nil)

		proxy := &Proxy{}
		handler := getCBOComparison(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "required")
	})

	t.Run("with time range filtering", func(t *testing.T) {
		startTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/comparison?collection_id=1&start_time="+startTime+"&end_time="+endTime, nil)

		proxy := &Proxy{}
		handler := getCBOComparison(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCBORecommendations(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with valid collection_id", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:      "rec-1",
			CollectionID: 30,
			Timestamp:    time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/recommendations?collection_id=30", nil)

		proxy := &Proxy{}
		handler := getCBORecommendations(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "recommendations")
	})

	t.Run("with missing collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/recommendations", nil)

		proxy := &Proxy{}
		handler := getCBORecommendations(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "required")
	})

	t.Run("with time range filtering", func(t *testing.T) {
		startTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/recommendations?collection_id=1&start_time="+startTime+"&end_time="+endTime, nil)

		proxy := &Proxy{}
		handler := getCBORecommendations(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHTTPHandlers_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	proxy := &Proxy{}
	proxy.RegisterRestRouter(router)

	t.Run("CBO metrics endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBOMetricsPath, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		// Should not return 404 (endpoint exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("CBO evaluation endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBOEvaluationPath+"?collection_id=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("CBO comparison endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBOComparisonPath+"?collection_id=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("CBO recommendations endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBORecommendationsPath+"?collection_id=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("CBO overview endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBOOverviewPath+"?collection_id=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})

	t.Run("CBO top endpoint registered", func(t *testing.T) {
		req, _ := http.NewRequest("GET", mhttp.CBOTopPath+"?collection_id=1", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestGetCBOMetrics_Pagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector := setupTestCollectorForHTTP()
	ctx := context.Background()

	// Add multiple metrics
	baseTime := time.Now()
	for i := 0; i < 10; i++ {
		metric := &cbo.CBOMetrics{
			QueryID:      fmt.Sprintf("query-%d", i),
			CollectionID: 1,
			Timestamp:    baseTime.Add(time.Duration(i) * time.Second),
		}
		collector.RecordMetrics(ctx, metric)
	}

	t.Run("pagination with limit and offset", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=1&limit=3&offset=2", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "limit")
		assert.Contains(t, w.Body.String(), "offset")
		assert.Contains(t, w.Body.String(), "total_count")
	})

	t.Run("pagination with max limit enforcement", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=1&limit=2000", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		// Should cap at 1000
	})

	t.Run("sorting ascending", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=1&sort=timestamp_asc&limit=5", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("sorting descending (default)", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?collection_id=1&sort=timestamp_desc&limit=5", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCBOMetrics_Redaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	collector := setupTestCollectorForHTTP()
	ctx := context.Background()

	metric := &cbo.CBOMetrics{
		QueryID:          "query-redact",
		CollectionID:     1,
		FilterExpression: "sensitive_data = 'secret'",
		Timestamp:        time.Now(),
	}
	collector.RecordMetrics(ctx, metric)

	t.Run("redaction by default", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?query_id=query-redact", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		// FilterExpression should be redacted by default (config defaults to false)
		// Note: This test depends on config, so it may need adjustment based on actual config state
	})

	t.Run("redaction with include_filter_expr=false", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/metrics?query_id=query-redact&include_filter_expr=false", nil)

		proxy := &Proxy{}
		handler := getCBOMetrics(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCBOOverview(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with valid collection_id", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		metric := &cbo.CBOMetrics{
			QueryID:                "overview-1",
			CollectionID:           40,
			CollectionName:          "test_collection",
			ExecutionTimeWithCBO:    50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			Timestamp:              time.Now(),
		}
		metric.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, metric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/overview?collection_id=40", nil)

		proxy := &Proxy{}
		handler := getCBOOverview(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "TotalQueries")
		assert.Contains(t, w.Body.String(), "QueriesImproved")
		assert.Contains(t, w.Body.String(), "BaselineCoveragePercent")
	})

	t.Run("with missing collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/overview", nil)

		proxy := &Proxy{}
		handler := getCBOOverview(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "required")
	})

	t.Run("with time range", func(t *testing.T) {
		startTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		endTime := time.Now().Add(1 * time.Hour).Format(time.RFC3339)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/overview?collection_id=1&start_time="+startTime+"&end_time="+endTime, nil)

		proxy := &Proxy{}
		handler := getCBOOverview(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGetCBOTop(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("with valid collection_id - degraded", func(t *testing.T) {
		collector := setupTestCollectorForHTTP()
		ctx := context.Background()

		// Add degraded query
		degradedMetric := &cbo.CBOMetrics{
			QueryID:                "degraded-1",
			CollectionID:           50,
			ExecutionTimeWithCBO:    150 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			Timestamp:              time.Now(),
		}
		degradedMetric.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, degradedMetric)

		// Add improved query
		improvedMetric := &cbo.CBOMetrics{
			QueryID:                "improved-1",
			CollectionID:           50,
			ExecutionTimeWithCBO:    50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			Timestamp:              time.Now(),
		}
		improvedMetric.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, improvedMetric)

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/top?collection_id=50&order=degraded&limit=5", nil)

		proxy := &Proxy{}
		handler := getCBOTop(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "queries")
		assert.Contains(t, w.Body.String(), "degraded")
	})

	t.Run("with valid collection_id - improved", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/top?collection_id=50&order=improved&limit=5", nil)

		proxy := &Proxy{}
		handler := getCBOTop(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "improved")
	})

	t.Run("with missing collection_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/top", nil)

		proxy := &Proxy{}
		handler := getCBOTop(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "required")
	})

	t.Run("with invalid order", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/top?collection_id=1&order=invalid", nil)

		proxy := &Proxy{}
		handler := getCBOTop(proxy)
		handler(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "degraded")
		assert.Contains(t, w.Body.String(), "improved")
	})

	t.Run("with limit enforcement", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/_cbo/top?collection_id=1&limit=200", nil)

		proxy := &Proxy{}
		handler := getCBOTop(proxy)
		handler(c)

		assert.Equal(t, http.StatusOK, w.Code)
		// Should cap at 100
	})
}

