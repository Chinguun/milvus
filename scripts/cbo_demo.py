#!/usr/bin/env python3
"""
CBO Customization Demonstration Script

This script demonstrates the Cost-Based Optimizer (CBO) customization in Milvus by:
1. Checking if Milvus is ready and healthy
2. Creating a collection with vector and scalar fields
3. Inserting vectors with varied, realistic data distributions
4. Executing diverse search queries with different filter expressions to trigger CBO
5. Retrieving and displaying comprehensive CBO evaluation metrics via HTTP endpoints
6. Generating detailed reports (JSON/Markdown) for analysis
"""

import argparse
import json
import random
import sys
import time
from datetime import datetime
from typing import List, Optional, Dict, Any, Tuple

import numpy as np
import requests
from pymilvus import (
    connections,
    utility,
    FieldSchema,
    CollectionSchema,
    DataType,
    Collection,
)

# Default configuration
DEFAULT_MILVUS_HOST = "127.0.0.1"
DEFAULT_MILVUS_PORT = "19530"
DEFAULT_HTTP_PORT = 9091
DEFAULT_COLLECTION_NAME = "cbo_demo_collection"
DEFAULT_NUM_VECTORS = 100000
DEFAULT_DIM = 128
DEFAULT_BATCH_SIZE = 10000
DEFAULT_SEED = 42
DEFAULT_INDEX_TYPE = "IVF_FLAT"
DEFAULT_METRIC_TYPE = "L2"
DEFAULT_NLIST = 128
DEFAULT_NPROBE = 10
DEFAULT_TOPK = 10
DEFAULT_WARMUP_QUERIES = 3

# CBO HTTP endpoints (under /api/v1 prefix)
API_PREFIX = "/api/v1"
CBO_METRICS_PATH = f"{API_PREFIX}/_cbo/metrics"
CBO_EVALUATION_PATH = f"{API_PREFIX}/_cbo/evaluation"
CBO_COMPARISON_PATH = f"{API_PREFIX}/_cbo/comparison"
CBO_RECOMMENDATIONS_PATH = f"{API_PREFIX}/_cbo/recommendations"
CBO_OVERVIEW_PATH = f"{API_PREFIX}/_cbo/overview"
CBO_TOP_PATH = f"{API_PREFIX}/_cbo/top"
HEALTHZ_PATH = "/healthz"
LIVEZ_PATH = "/livez"


class CBODemo:
    def __init__(
        self,
        milvus_host: str = DEFAULT_MILVUS_HOST,
        milvus_port: str = DEFAULT_MILVUS_PORT,
        http_port: int = DEFAULT_HTTP_PORT,
        collection_name: str = DEFAULT_COLLECTION_NAME,
        num_vectors: int = DEFAULT_NUM_VECTORS,
        dim: int = DEFAULT_DIM,
        batch_size: int = DEFAULT_BATCH_SIZE,
        seed: Optional[int] = DEFAULT_SEED,
        index_type: str = DEFAULT_INDEX_TYPE,
        metric_type: str = DEFAULT_METRIC_TYPE,
        nlist: int = DEFAULT_NLIST,
        nprobe: int = DEFAULT_NPROBE,
        topk: int = DEFAULT_TOPK,
        warmup_queries: int = DEFAULT_WARMUP_QUERIES,
        report_json: Optional[str] = None,
        report_md: Optional[str] = None,
    ):
        self.milvus_host = milvus_host
        self.milvus_port = milvus_port
        self.http_port = http_port
        self.collection_name = collection_name
        self.num_vectors = num_vectors
        self.dim = dim
        self.batch_size = batch_size
        self.seed = seed
        self.index_type = index_type
        self.metric_type = metric_type
        self.nlist = nlist
        self.nprobe = nprobe
        self.topk = topk
        self.warmup_queries = warmup_queries
        self.report_json = report_json
        self.report_md = report_md
        self.collection: Optional[Collection] = None
        self.collection_id: Optional[int] = None
        self.http_base_url = f"http://{milvus_host}:{http_port}"
        self.query_results: List[Dict[str, Any]] = []
        self.cbo_metrics: List[Dict[str, Any]] = []
        self.evaluation_summary: Optional[Dict[str, Any]] = None
        self.comparison: Optional[Dict[str, Any]] = None
        self.recommendations: Optional[List[str]] = None

        # Initialize random seed for reproducibility
        if seed is not None:
            random.seed(seed)
            np.random.seed(seed)

    def print_config(self) -> None:
        """Print configuration banner."""
        print("\n" + "=" * 80)
        print("CBO Customization Demonstration - Configuration")
        print("=" * 80)
        print(f"Collection Name:        {self.collection_name}")
        print(f"Number of Vectors:      {self.num_vectors:,}")
        print(f"Vector Dimension:       {self.dim}")
        print(f"Batch Size:             {self.batch_size:,}")
        print(f"Random Seed:            {self.seed}")
        print(f"Milvus Address:         {self.milvus_host}:{self.milvus_port}")
        print(f"HTTP API:               {self.http_base_url}")
        print(f"Index Type:             {self.index_type}")
        print(f"Metric Type:            {self.metric_type}")
        print(f"Index Params (nlist):    {self.nlist}")
        print(f"Search Params (nprobe):  {self.nprobe}")
        print(f"Top-K:                   {self.topk}")
        print(f"Warmup Queries:          {self.warmup_queries}")
        if self.report_json:
            print(f"JSON Report:            {self.report_json}")
        if self.report_md:
            print(f"Markdown Report:         {self.report_md}")
        print("=" * 80)
        print("\n📌 Note: Baseline comparison is enabled by default with 100% sampling rate (for demo).")
        print("   All queries will have baseline comparison data.")
        print("   For production, consider reducing proxy.cbo.evaluation.baselineSampleRate to 0.01 (1%)")
        print("   to limit overhead from executing queries twice.")
        print("=" * 80)

    def check_milvus_ready(self, max_retries: int = 30, retry_interval: float = 2.0) -> bool:
        """
        Check if Milvus is ready and healthy.
        Returns True if Milvus is ready, False otherwise.
        """
        print("\n" + "=" * 80)
        print("Step 1: Checking Milvus Readiness")
        print("=" * 80)

        # Check HTTP health endpoint
        health_url = f"{self.http_base_url}{HEALTHZ_PATH}"
        print(f"Checking HTTP health endpoint: {health_url}")

        for attempt in range(max_retries):
            try:
                response = requests.get(health_url, timeout=5)
                if response.status_code == 200:
                    print(f"✓ Milvus HTTP endpoint is healthy (status: {response.status_code})")
                    break
                else:
                    print(f"  Attempt {attempt + 1}/{max_retries}: HTTP status {response.status_code}, retrying...")
            except requests.exceptions.RequestException as e:
                if attempt < max_retries - 1:
                    print(f"  Attempt {attempt + 1}/{max_retries}: Connection failed: {e}, retrying in {retry_interval}s...")
                    time.sleep(retry_interval)
                else:
                    print(f"✗ Failed to connect to Milvus HTTP endpoint after {max_retries} attempts")
                    return False
        else:
            print(f"✗ Milvus HTTP endpoint not ready after {max_retries} attempts")
            return False

        # Check pymilvus connection
        print(f"Checking pymilvus connection: {self.milvus_host}:{self.milvus_port}")
        try:
            connections.connect(
                alias="default",
                host=self.milvus_host,
                port=self.milvus_port,
            )
            print("✓ Successfully connected to Milvus via pymilvus")

            # Verify connection by listing collections
            collections = utility.list_collections()
            print(f"✓ Milvus is ready. Found {len(collections)} existing collection(s)")
            return True
        except Exception as e:
            print(f"✗ Failed to connect to Milvus via pymilvus: {e}")
            return False

    def setup_collection(self, drop_existing: bool = True) -> bool:
        """
        Create collection with schema for CBO demonstration.
        """
        print("\n" + "=" * 80)
        print("Step 2: Setting up Collection")
        print("=" * 80)

        try:
            # Drop existing collection if requested
            if drop_existing and utility.has_collection(self.collection_name):
                print(f"Dropping existing collection: {self.collection_name}")
                utility.drop_collection(self.collection_name)
                print("✓ Collection dropped")

            # Define schema
            print(f"Creating collection: {self.collection_name}")
            fields = [
                FieldSchema(name="id", dtype=DataType.INT64, is_primary=True, auto_id=True),
                FieldSchema(name="price", dtype=DataType.FLOAT, description="Price field for filtering"),
                FieldSchema(name="category", dtype=DataType.INT64, description="Category field for filtering"),
                FieldSchema(name="rating", dtype=DataType.FLOAT, description="Rating field for range queries"),
                FieldSchema(
                    name="vector",
                    dtype=DataType.FLOAT_VECTOR,
                    dim=self.dim,
                    description="Vector field for similarity search",
                ),
            ]
            schema = CollectionSchema(
                fields=fields,
                description="CBO demonstration collection with vector and scalar fields",
            )

            # Create collection
            self.collection = Collection(
                name=self.collection_name,
                schema=schema,
            )
            print("✓ Collection created successfully")
            print("✓ Collection setup complete")
            return True

        except Exception as e:
            print(f"✗ Failed to setup collection: {e}")
            import traceback
            traceback.print_exc()
            return False

    def generate_data_batch(self, batch_size: int) -> Tuple[List[float], List[int], List[float], List[List[float]]]:
        """
        Generate a batch of data with realistic distributions optimized for CBO testing.
        
        Returns:
            Tuple of (prices, categories, ratings, vectors)
        """
        # Price: Log-normal distribution (long-tail, many small values, few huge)
        # This creates varied selectivity - filtering on low prices is very selective,
        # filtering on high prices is less selective
        log_mean = 4.0  # Mean of underlying normal distribution
        log_std = 1.2   # Std dev of underlying normal distribution
        prices = np.clip(np.random.lognormal(log_mean, log_std, batch_size), 0.01, 10000.0).tolist()

        # Category: Zipf-like distribution (skewed, hot categories)
        # Categories 1-3 are very common, 4-7 are moderate, 8-10 are rare
        # This tests IN/== selectivity with different category frequencies
        category_probs = [0.25, 0.20, 0.15, 0.12, 0.10, 0.08, 0.05, 0.03, 0.02, 0.00]  # 10 categories
        categories = np.random.choice(range(1, 11), size=batch_size, p=category_probs).astype(int).tolist()

        # Rating: Clustered bands (bimodal distribution)
        # Most items cluster around 2.0-2.5 (low quality) and 4.0-4.5 (high quality)
        # This creates interesting selectivity patterns for range queries
        band1 = np.random.normal(2.2, 0.3, batch_size // 2)
        band2 = np.random.normal(4.3, 0.3, batch_size - batch_size // 2)
        ratings = np.clip(np.concatenate([band1, band2]), 0.0, 5.0).tolist()
        np.random.shuffle(ratings)  # Shuffle to mix the bands

        # Vectors: Random float vectors using numpy for performance
        vectors = np.random.rand(batch_size, self.dim).astype(np.float32).tolist()

        return prices, categories, ratings, vectors

    def insert_data(self) -> bool:
        """
        Insert sample data with varied distributions to trigger different CBO decisions.
        Uses numpy for efficient batch generation.
        """
        print("\n" + "=" * 80)
        print("Step 3: Inserting Data")
        print("=" * 80)

        try:
            num_batches = (self.num_vectors + self.batch_size - 1) // self.batch_size
            print(f"Inserting {self.num_vectors:,} vectors in {num_batches} batches (batch size: {self.batch_size:,})")
            print("Data distributions:")
            print("  - Price: Log-normal (long-tail distribution)")
            print("  - Category: Zipf-like (skewed, hot categories)")
            print("  - Rating: Bimodal (clustered around 2.2 and 4.3)")

            total_inserted = 0
            start_time = time.time()

            for batch_idx in range(num_batches):
                batch_start = batch_idx * self.batch_size
                batch_end = min(batch_start + self.batch_size, self.num_vectors)
                batch_size_actual = batch_end - batch_start

                # Generate data batch
                prices, categories, ratings, vectors = self.generate_data_batch(batch_size_actual)

                # Insert batch
                self.collection.insert([prices, categories, ratings, vectors])
                total_inserted += batch_size_actual

                if (batch_idx + 1) % 10 == 0 or batch_idx == num_batches - 1:
                    elapsed = time.time() - start_time
                    rate = total_inserted / elapsed if elapsed > 0 else 0
                    print(
                        f"  Progress: {total_inserted:,}/{self.num_vectors:,} vectors "
                        f"({100 * total_inserted / self.num_vectors:.1f}%) - "
                        f"{rate:,.0f} vectors/sec"
                    )

            # Flush to ensure data is persisted
            print("Flushing collection...")
            self.collection.flush()
            print(f"✓ Successfully inserted {total_inserted:,} vectors")

            # Get collection stats
            num_entities = self.collection.num_entities
            print(f"✓ Collection now contains {num_entities:,} entities")
            return True

        except Exception as e:
            print(f"✗ Failed to insert data: {e}")
            import traceback
            traceback.print_exc()
            return False

    def create_index_and_load(self, skip_index: bool = False, skip_load: bool = False) -> bool:
        """
        Create index on vector field and load collection.
        """
        print("\n" + "=" * 80)
        print("Step 4: Creating Index and Loading Collection")
        print("=" * 80)

        try:
            if skip_index:
                print("Skipping index creation (--no-index)")
            elif self.collection.has_index():
                print("Collection already has index")
            else:
                print(f"Creating {self.index_type} index on vector field...")
                index_params = {
                    "index_type": self.index_type,
                    "metric_type": self.metric_type,
                    "params": {"nlist": self.nlist},
                }
                self.collection.create_index(field_name="vector", index_params=index_params)
                print(f"✓ Index created successfully ({self.index_type}, {self.metric_type}, nlist={self.nlist})")

            if skip_load:
                print("Skipping collection load (--no-load)")
            else:
                print("Loading collection...")
                self.collection.load()
                print("✓ Collection loaded successfully")
            return True

        except Exception as e:
            print(f"✗ Failed to create index or load collection: {e}")
            import traceback
            traceback.print_exc()
            return False

    def get_query_templates(self) -> List[Dict[str, Any]]:
        """
        Get comprehensive query templates covering different selectivity patterns.
        """
        queries = [
            # High selectivity queries (should use standard filtering)
            {
                "name": "High Selectivity - Very Low Price",
                "filter": "price < 10",
                "description": "Very selective query (<1% of data), should use standard filtering",
                "expected_strategy": "standard_filter",
            },
            {
                "name": "High Selectivity - Exact Price Match",
                "filter": "price == 100",
                "description": "Exact match, very selective, should use standard filtering",
                "expected_strategy": "standard_filter",
            },
            {
                "name": "High Selectivity - Rare Category",
                "filter": "category == 9",
                "description": "Rare category (~2% of data), should use standard filtering",
                "expected_strategy": "standard_filter",
            },
            {
                "name": "High Selectivity - Very High Rating",
                "filter": "rating > 4.8",
                "description": "Very high rating, selective, should use standard filtering",
                "expected_strategy": "standard_filter",
            },
            # Medium selectivity queries
            {
                "name": "Medium Selectivity - Price Range",
                "filter": "price >= 50 && price <= 200",
                "description": "Moderate price range, medium selectivity",
                "expected_strategy": "iterative_filter",
            },
            {
                "name": "Medium Selectivity - Common Categories",
                "filter": "category in [1, 2, 3]",
                "description": "Common categories (~60% of data), should use iterative filtering",
                "expected_strategy": "iterative_filter",
            },
            {
                "name": "Medium Selectivity - Rating Range",
                "filter": "rating >= 3.5 && rating <= 4.5",
                "description": "Rating range covering high-quality band",
                "expected_strategy": "iterative_filter",
            },
            # Low selectivity queries (should use iterative filtering)
            {
                "name": "Low Selectivity - Wide Price Range",
                "filter": "price >= 100 && price <= 5000",
                "description": "Wide price range, low selectivity, should use iterative filtering",
                "expected_strategy": "iterative_filter",
            },
            {
                "name": "Low Selectivity - Many Categories",
                "filter": "category in [1, 2, 3, 4, 5, 6, 7]",
                "description": "Large IN list (~85% of data), should use iterative filtering",
                "expected_strategy": "iterative_filter",
            },
            {
                "name": "Low Selectivity - Low Rating Threshold",
                "filter": "rating > 1.5",
                "description": "Low threshold, high selectivity, should use iterative filtering",
                "expected_strategy": "iterative_filter",
            },
            # Combined predicates
            {
                "name": "Combined - Price AND Category",
                "filter": "price < 100 && category == 1",
                "description": "Combined filters, high selectivity",
                "expected_strategy": "standard_filter",
            },
            {
                "name": "Combined - Price AND Rating",
                "filter": "price >= 50 && price <= 200 && rating > 4.0",
                "description": "Combined filters, medium selectivity",
                "expected_strategy": "iterative_filter",
            },
            {
                "name": "Combined - Category OR Rating",
                "filter": "category == 1 || rating > 4.5",
                "description": "OR condition, low selectivity",
                "expected_strategy": "iterative_filter",
            },
            # Edge cases
            {
                "name": "Edge Case - Very Tight Range",
                "filter": "price >= 99.5 && price <= 100.5",
                "description": "Very tight range, high selectivity",
                "expected_strategy": "standard_filter",
            },
            {
                "name": "Edge Case - All Categories",
                "filter": "category in [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]",
                "description": "All categories (effectively no filter), should use iterative",
                "expected_strategy": "iterative_filter",
            },
        ]
        return queries

    def execute_warmup_queries(self) -> None:
        """Execute warmup queries to stabilize caches."""
        if self.warmup_queries <= 0:
            return

        print(f"\nExecuting {self.warmup_queries} warmup queries to stabilize caches...")
        query_vectors = np.random.rand(self.warmup_queries, self.dim).astype(np.float32).tolist()
        search_params = {"metric_type": self.metric_type, "params": {"nprobe": self.nprobe}}

        for i in range(self.warmup_queries):
            try:
                self.collection.search(
                    data=[query_vectors[i]],
                    anns_field="vector",
                    param=search_params,
                    limit=self.topk,
                    timeout=30,
                )
            except Exception:
                pass  # Ignore warmup errors
        print("✓ Warmup queries completed")

    def execute_queries(self) -> List[Dict[str, Any]]:
        """
        Execute search queries with different filter expressions to trigger CBO.
        """
        print("\n" + "=" * 80)
        print("Step 5: Executing Search Queries")
        print("=" * 80)

        queries = self.get_query_templates()
        print(f"Executing {len(queries)} queries with diverse selectivity patterns...")

        # Generate query vectors
        query_vectors = np.random.rand(len(queries), self.dim).astype(np.float32).tolist()
        search_params = {"metric_type": self.metric_type, "params": {"nprobe": self.nprobe}}

        results = []
        successful_queries = 0

        for i, query in enumerate(queries):
            try:
                print(f"\n  Query {i + 1}/{len(queries)}: {query['name']}")
                print(f"    Filter: {query['filter']}")
                print(f"    Description: {query['description']}")

                start_time = time.time()
                search_results = self.collection.search(
                    data=[query_vectors[i]],
                    anns_field="vector",
                    param=search_params,
                    limit=self.topk,
                    expr=query["filter"],
                    output_fields=["price", "category", "rating"],
                    timeout=60,
                )
                elapsed_time = time.time() - start_time

                num_results = sum(len(hits) for hits in search_results)
                print(f"    ✓ Completed in {elapsed_time:.3f}s, found {num_results} results")

                results.append(
                    {
                        "query": query,
                        "elapsed_time": elapsed_time,
                        "num_results": num_results,
                        "timestamp": time.time(),
                        "success": True,
                    }
                )
                successful_queries += 1

                # Small delay to ensure metrics are recorded
                time.sleep(0.3)

            except Exception as e:
                print(f"    ✗ Query failed: {e}")
                results.append(
                    {
                        "query": query,
                        "elapsed_time": None,
                        "num_results": 0,
                        "error": str(e),
                        "timestamp": time.time(),
                        "success": False,
                    }
                )

        print(f"\n✓ Completed {successful_queries}/{len(queries)} queries successfully")
        self.query_results = results
        return results

    def query_cbo_metrics(self, collection_id: Optional[int] = None, limit: int = 1000) -> List[Dict[str, Any]]:
        """
        Query CBO metrics endpoint.
        """
        try:
            url = f"{self.http_base_url}{CBO_METRICS_PATH}"
            params = {"limit": limit, "include_filter_expr": "true"}
            if collection_id:
                params["collection_id"] = collection_id

            response = requests.get(url, params=params, timeout=10)
            if response.status_code == 200:
                data = response.json()
                if "data" in data and "metrics" in data["data"]:
                    return data["data"]["metrics"]
                return data.get("data", [])
            else:
                print(f"Warning: CBO metrics endpoint returned status {response.status_code}")
                return []
        except Exception as e:
            print(f"Warning: Failed to query CBO metrics: {e}")
            return []

    def query_cbo_evaluation(self, collection_id: int) -> Optional[Dict[str, Any]]:
        """
        Query CBO evaluation summary endpoint.
        """
        try:
            url = f"{self.http_base_url}{CBO_EVALUATION_PATH}"
            params = {"collection_id": collection_id}

            response = requests.get(url, params=params, timeout=10)
            if response.status_code == 200:
                data = response.json()
                return data.get("data")
            else:
                print(f"Warning: CBO evaluation endpoint returned status {response.status_code}")
                return None
        except Exception as e:
            print(f"Warning: Failed to query CBO evaluation: {e}")
            return None

    def query_cbo_comparison(self, collection_id: int) -> Optional[Dict[str, Any]]:
        """
        Query CBO comparison endpoint.
        """
        try:
            url = f"{self.http_base_url}{CBO_COMPARISON_PATH}"
            params = {"collection_id": collection_id}

            response = requests.get(url, params=params, timeout=10)
            if response.status_code == 200:
                data = response.json()
                return data.get("data")
            else:
                print(f"Warning: CBO comparison endpoint returned status {response.status_code}")
                return None
        except Exception as e:
            print(f"Warning: Failed to query CBO comparison: {e}")
            return None

    def query_cbo_recommendations(self, collection_id: int) -> Optional[List[str]]:
        """
        Query CBO recommendations endpoint.
        """
        try:
            url = f"{self.http_base_url}{CBO_RECOMMENDATIONS_PATH}"
            params = {"collection_id": collection_id}

            response = requests.get(url, params=params, timeout=10)
            if response.status_code == 200:
                data = response.json()
                if "data" in data and "recommendations" in data["data"]:
                    return data["data"]["recommendations"]
                return []
            else:
                print(f"Warning: CBO recommendations endpoint returned status {response.status_code}")
                return None
        except Exception as e:
            print(f"Warning: Failed to query CBO recommendations: {e}")
            return None

    def query_cbo_top(self, collection_id: int, order: str = "degraded", limit: int = 10) -> Optional[List[Dict[str, Any]]]:
        """
        Query CBO top-N queries endpoint.
        """
        try:
            url = f"{self.http_base_url}{CBO_TOP_PATH}"
            params = {"collection_id": collection_id, "order": order, "limit": limit}

            response = requests.get(url, params=params, timeout=10)
            if response.status_code == 200:
                data = response.json()
                # Handler returns {"data": {"queries": [...], "count": ..., "order": ...}}
                if "data" in data and isinstance(data["data"], dict):
                    return data["data"].get("queries", [])
                return []
            else:
                return None
        except Exception as e:
            return None

    def get_field(self, metric: Dict[str, Any], *possible_names: str) -> Any:
        """Get field value trying different possible field name formats."""
        for name in possible_names:
            value = metric.get(name)
            if value is not None:
                return value
        return None

    def find_collection_id(self, all_metrics: List[Dict[str, Any]]) -> Optional[int]:
        """Find collection ID from metrics, preferring the one matching our collection name."""
        collection_ids = {}
        for metric in all_metrics:
            coll_id = self.get_field(metric, "CollectionID", "collectionID", "collection_id")
            coll_name = self.get_field(metric, "CollectionName", "collectionName", "collection_name")
            if coll_id:
                if coll_id not in collection_ids:
                    collection_ids[coll_id] = []
                if coll_name:
                    collection_ids[coll_id].append(coll_name)

        # Prefer collection ID matching our collection name
        for coll_id, names in collection_ids.items():
            if self.collection_name in names:
                return coll_id

        # Fall back to first collection ID found
        if collection_ids:
            return list(collection_ids.keys())[0]

        return None

    def check_baseline_config(self) -> None:
        """
        Check and warn about baseline comparison configuration.
        """
        print("\n" + "-" * 80)
        print("Checking Baseline Comparison Configuration")
        print("-" * 80)
        
        try:
            # Try to get metrics to check baseline status
            all_metrics = self.query_cbo_metrics()
            if not all_metrics:
                print("⚠ No metrics found yet. Baseline check will be performed after queries execute.")
                return
            
            # Count metrics with baseline data
            metrics_with_baseline = 0
            metrics_without_baseline = 0
            for metric in all_metrics:
                exec_time_without = self.get_field(metric, "ExecutionTimeWithoutCBO", "executionTimeWithoutCBO", "execution_time_without_cbo")
                baseline_status = self.get_field(metric, "BaselineStatus", "baselineStatus", "baseline_status")
                if exec_time_without and exec_time_without > 0:
                    metrics_with_baseline += 1
                elif baseline_status == "not_sampled":
                    metrics_without_baseline += 1
            
            total_metrics = len(all_metrics)
            baseline_coverage = (metrics_with_baseline / total_metrics * 100) if total_metrics > 0 else 0
            
            print(f"Total Metrics: {total_metrics}")
            print(f"Metrics with Baseline: {metrics_with_baseline}")
            print(f"Baseline Coverage: {baseline_coverage:.1f}%")
            
            if baseline_coverage < 10 and total_metrics > 5:
                print("\n⚠ WARNING: Low baseline coverage detected!")
                print("   Baseline comparison may be disabled or sample rate is too low.")
                print("\n   To enable/configure baseline comparison:")
                print("   1. Set proxy.cbo.evaluation.baselineComparison=true (default: true)")
                print("   2. Increase proxy.cbo.evaluation.baselineSampleRate (default: 0.01 = 1%)")
                print("      For demo purposes, consider setting it to 0.5 (50%) or 1.0 (100%)")
                print("\n   Configuration methods:")
                print("   - Via config file: Add to milvus.yaml under proxy section")
                print("   - Via environment: export MILVUS_PROXY_CBO_EVALUATION_BASELINECOMPARISON=true")
                print("                     export MILVUS_PROXY_CBO_EVALUATION_BASELINESAMPLERATE=0.5")
                print("   - Restart Milvus proxy after configuration changes")
            elif baseline_coverage == 0 and total_metrics > 0:
                print("\n⚠ WARNING: No baseline data found!")
                print("   Baseline comparison appears to be disabled.")
                print("   Enable it by setting proxy.cbo.evaluation.baselineComparison=true")
            else:
                print("✓ Baseline comparison is working correctly")
        except Exception as e:
            print(f"⚠ Could not check baseline configuration: {e}")

    def display_results(self, query_results: List[Dict[str, Any]]) -> None:
        """
        Display comprehensive CBO evaluation results with improved formatting.
        """
        print("\n" + "=" * 80)
        print("Step 6: CBO Evaluation Results")
        print("=" * 80)

        # Wait a bit for metrics to be recorded
        print("Waiting for CBO metrics to be recorded...")
        time.sleep(3)
        
        # Check baseline configuration
        self.check_baseline_config()

        # Get all metrics first to find collection ID
        all_metrics = self.query_cbo_metrics()
        if not all_metrics:
            print("\n⚠ No CBO metrics found. Make sure:")
            print("  - CBO evaluation is enabled (proxy.cbo.evaluation.enabled=true)")
            print("  - Queries have been executed")
            print("  - Milvus proxy is accessible on HTTP port")
            return
        
        # Check baseline coverage
        metrics_with_baseline = sum(1 for m in all_metrics 
                                    if self.get_field(m, "ExecutionTimeWithoutCBO", "executionTimeWithoutCBO", "execution_time_without_cbo") 
                                    and self.get_field(m, "ExecutionTimeWithoutCBO", "executionTimeWithoutCBO", "execution_time_without_cbo") > 0)
        baseline_coverage = (metrics_with_baseline / len(all_metrics) * 100) if all_metrics else 0
        
        if baseline_coverage < 10 and len(all_metrics) > 5:
            print(f"\n⚠ WARNING: Only {baseline_coverage:.1f}% of queries have baseline data.")
            print("   This is likely due to low baseline sample rate (default: 1%).")
            print("   To see baseline comparisons in the dashboard:")
            print("   - Increase proxy.cbo.evaluation.baselineSampleRate (e.g., 0.5 for 50%)")
            print("   - Or set to 1.0 for 100% baseline coverage (for demo purposes)")
            print("   - Restart Milvus proxy after configuration changes")
            print()

        # Find collection ID
        collection_id = self.find_collection_id(all_metrics)
        if not collection_id:
            print("\n⚠ Could not determine collection ID from metrics")
            print("Available metrics (first metric sample):")
            if all_metrics:
                print(f"  {json.dumps(all_metrics[0], indent=2, default=str)}")
            return

        print(f"\nFound collection ID: {collection_id}")

        # Filter metrics for this collection
        collection_metrics = []
        for m in all_metrics:
            coll_id = self.get_field(m, "CollectionID", "collectionID", "collection_id")
            coll_name = self.get_field(m, "CollectionName", "collectionName", "collection_name")
            if coll_id == collection_id:
                if not coll_name or coll_name == self.collection_name:
                    collection_metrics.append(m)

        if not collection_metrics:
            # Try without name filter
            for m in all_metrics:
                coll_id = self.get_field(m, "CollectionID", "collectionID", "collection_id")
                if coll_id == collection_id:
                    collection_metrics.append(m)

        print(f"Found {len(collection_metrics)} CBO metrics for collection {self.collection_name}")
        self.cbo_metrics = collection_metrics

        # Fetch evaluation data
        self.evaluation_summary = self.query_cbo_evaluation(collection_id)
        self.comparison = self.query_cbo_comparison(collection_id)
        self.recommendations = self.query_cbo_recommendations(collection_id)
        self.collection_id = collection_id

        # Display summary table
        self.display_summary_table()

        # Display individual metrics
        self.display_individual_metrics(collection_metrics)

        # Display evaluation summary
        self.display_evaluation_summary()

        # Display comparison
        self.display_comparison()

        # Display top queries
        self.display_top_queries(collection_id)

        # Display recommendations
        self.display_recommendations()

        # Generate reports if requested
        if self.report_json or self.report_md:
            self.generate_reports()

        print("\n" + "=" * 80)
        print("CBO Demonstration Complete!")
        print("=" * 80)
        self.print_final_summary()

    def display_summary_table(self) -> None:
        """Display a summary table of CBO metrics."""
        print("\n" + "-" * 80)
        print("CBO Summary Table")
        print("-" * 80)

        if not self.evaluation_summary:
            print("⚠ Could not retrieve evaluation summary")
            return

        total_queries = self.evaluation_summary.get("TotalQueries", 0)
        queries_with_cbo = self.evaluation_summary.get("QueriesWithCBO", 0)
        queries_improved = self.evaluation_summary.get("QueriesImproved", 0)
        queries_degraded = self.evaluation_summary.get("QueriesDegraded", 0)
        queries_neutral = self.evaluation_summary.get("QueriesNeutral", 0)

        print(f"{'Metric':<40} {'Value':>20}")
        print("-" * 60)
        print(f"{'Total Queries':<40} {total_queries:>20,}")
        print(f"{'Queries with CBO':<40} {queries_with_cbo:>20,}")
        print(f"{'Queries Improved':<40} {queries_improved:>20,}")
        print(f"{'Queries Degraded':<40} {queries_degraded:>20,}")
        print(f"{'Queries Neutral':<40} {queries_neutral:>20,}")

        avg_reduction = self.evaluation_summary.get("AvgTimeReductionPercent", 0)
        print(f"{'Avg Time Reduction %':<40} {avg_reduction:>19.2f}%")

        strategy_dist = self.evaluation_summary.get("StrategyDistribution", {})
        if strategy_dist:
            print(f"\n{'Strategy Distribution':<40}")
            for strategy, count in sorted(strategy_dist.items()):
                print(f"  {strategy:<38} {count:>20,}")

        estimator_dist = self.evaluation_summary.get("EstimatorDistribution", {})
        if estimator_dist:
            print(f"\n{'Estimator Distribution':<40}")
            for estimator, count in sorted(estimator_dist.items()):
                print(f"  {estimator:<38} {count:>20,}")

    def display_individual_metrics(self, metrics: List[Dict[str, Any]], limit: int = 15) -> None:
        """Display individual query metrics."""
        print("\n" + "-" * 80)
        print(f"Individual Query Metrics (showing top {limit})")
        print("-" * 80)

        # Sort by execution time (descending) to show slowest queries
        sorted_metrics = sorted(
            metrics,
            key=lambda m: self.get_field(m, "ExecutionTimeWithCBO", "executionTimeWithCBO", "execution_time_with_cbo") or 0,
            reverse=True,
        )

        for i, metric in enumerate(sorted_metrics[:limit]):
            print(f"\nQuery {i + 1}:")
            query_id = self.get_field(metric, "QueryID", "queryID", "query_id") or "N/A"
            filter_expr = self.get_field(metric, "FilterExpression", "filterExpression", "filter_expression") or "N/A"
            selectivity = self.get_field(metric, "SelectivityEstimate", "selectivityEstimate", "selectivity_estimate") or 0
            strategy = self.get_field(metric, "SelectedStrategy", "selectedStrategy", "selected_strategy") or "N/A"
            estimator = self.get_field(metric, "EstimatorType", "estimatorType", "estimator_type") or "N/A"
            exec_time = self.get_field(metric, "ExecutionTimeWithCBO", "executionTimeWithCBO", "execution_time_with_cbo") or 0
            opt_result = self.get_field(metric, "OptimizationResult", "optimizationResult", "optimization_result") or "N/A"

            print(f"  Query ID: {query_id}")
            print(f"  Filter: {filter_expr}")
            if isinstance(selectivity, (int, float)):
                print(f"  Selectivity: {selectivity:.4f}")
            else:
                print(f"  Selectivity: {selectivity}")
            print(f"  Strategy: {strategy}")
            print(f"  Estimator: {estimator}")
            if isinstance(exec_time, (int, float)) and exec_time > 0:
                exec_time_sec = exec_time / 1e9
                print(f"  Execution Time: {exec_time_sec:.3f}s")
            else:
                print("  Execution Time: N/A")
            print(f"  Result: {opt_result}")

    def display_evaluation_summary(self) -> None:
        """Display CBO evaluation summary."""
        print("\n" + "-" * 80)
        print("CBO Evaluation Summary")
        print("-" * 80)

        if not self.evaluation_summary:
            print("⚠ Could not retrieve evaluation summary")
            return

        # Already displayed in summary table, just show additional details
        # Calculate baseline coverage from comparison data if available
        baseline_coverage = 0
        if self.comparison:
            total_queries = self.comparison.get("TotalQueries", 0)
            queries_with_baseline = self.comparison.get("QueriesWithBaseline", 0)
            if total_queries > 0:
                baseline_coverage = (queries_with_baseline / total_queries) * 100
        
        if baseline_coverage > 0:
            print(f"Baseline Coverage: {baseline_coverage:.2f}%")

    def display_comparison(self) -> None:
        """Display CBO performance comparison."""
        print("\n" + "-" * 80)
        print("CBO Performance Comparison")
        print("-" * 80)

        if not self.comparison:
            print("⚠ Could not retrieve performance comparison")
            return

        total_queries = self.comparison.get("TotalQueries", 0)
        queries_with_baseline = self.comparison.get("QueriesWithBaseline", 0)
        avg_with_cbo = self.comparison.get("AvgExecutionTimeWithCBO", 0)
        avg_without_cbo = self.comparison.get("AvgExecutionTimeWithoutCBO", 0)
        avg_reduction = self.comparison.get("AvgTimeReductionPercent", 0)
        total_time_saved = self.comparison.get("TotalTimeSaved", 0)

        print(f"Total Queries: {total_queries:,}")
        print(f"Queries with Baseline: {queries_with_baseline:,}")

        if isinstance(avg_with_cbo, (int, float)) and avg_with_cbo > 0:
            avg_with_cbo_sec = avg_with_cbo / 1e9
            print(f"Average Execution Time (with CBO): {avg_with_cbo_sec:.3f}s")
        if isinstance(avg_without_cbo, (int, float)) and avg_without_cbo > 0:
            avg_without_cbo_sec = avg_without_cbo / 1e9
            print(f"Average Execution Time (without CBO): {avg_without_cbo_sec:.3f}s")
        print(f"Average Time Reduction: {avg_reduction:.2f}%")

        if isinstance(total_time_saved, (int, float)) and total_time_saved > 0:
            total_time_saved_sec = total_time_saved / 1e9
            print(f"Total Time Saved: {total_time_saved_sec:.2f}s")

        # Percentile metrics
        percentile_metrics = self.comparison.get("PercentileMetrics", {})
        if percentile_metrics:
            print("\nPercentile Time Reductions:")
            for percentile, reduction in percentile_metrics.items():
                if isinstance(reduction, (int, float)) and reduction > 0:
                    reduction_sec = reduction / 1e9
                    print(f"  {percentile}: {reduction_sec:.3f}s")

        opt_results = self.comparison.get("OptimizationResults", {})
        if opt_results:
            print(f"\nOptimization Results:")
            print(f"  Improved: {opt_results.get('Improved', 0):,}")
            print(f"  Degraded: {opt_results.get('Degraded', 0):,}")
            print(f"  Neutral: {opt_results.get('Neutral', 0):,}")

    def display_top_queries(self, collection_id: int) -> None:
        """Display top-N degraded and improved queries."""
        print("\n" + "-" * 80)
        print("Top Queries Analysis")
        print("-" * 80)

        degraded = self.query_cbo_top(collection_id, order="degraded", limit=5)
        improved = self.query_cbo_top(collection_id, order="improved", limit=5)

        if degraded:
            print("\nTop 5 Degraded Queries:")
            for i, q in enumerate(degraded, 1):
                filter_expr = self.get_field(q, "FilterExpression", "filterExpression", "filter_expression") or "N/A"
                exec_time = self.get_field(q, "ExecutionTimeWithCBO", "executionTimeWithCBO", "execution_time_with_cbo") or 0
                if isinstance(exec_time, (int, float)) and exec_time > 0:
                    exec_time_sec = exec_time / 1e9
                    print(f"  {i}. {filter_expr} ({exec_time_sec:.3f}s)")

        if improved:
            print("\nTop 5 Improved Queries:")
            for i, q in enumerate(improved, 1):
                filter_expr = self.get_field(q, "FilterExpression", "filterExpression", "filter_expression") or "N/A"
                reduction = self.get_field(q, "TimeReductionPercent", "timeReductionPercent", "time_reduction_percent") or 0
                print(f"  {i}. {filter_expr} ({reduction:.2f}% improvement)")

    def display_recommendations(self) -> None:
        """Display CBO recommendations."""
        print("\n" + "-" * 80)
        print("CBO Recommendations")
        print("-" * 80)

        if not self.recommendations:
            print("⚠ Could not retrieve recommendations")
            return

        if len(self.recommendations) == 0:
            print("No specific recommendations. CBO is performing well.")
        else:
            for i, rec in enumerate(self.recommendations, 1):
                print(f"{i}. {rec}")

    def generate_reports(self) -> None:
        """Generate JSON and/or Markdown reports."""
        report_data = {
            "timestamp": datetime.now().isoformat(),
            "collection_name": self.collection_name,
            "collection_id": self.collection_id,
            "num_vectors": self.num_vectors,
            "config": {
                "dim": self.dim,
                "index_type": self.index_type,
                "metric_type": self.metric_type,
                "nlist": self.nlist,
                "nprobe": self.nprobe,
                "topk": self.topk,
            },
            "query_results": self.query_results,
            "cbo_metrics": self.cbo_metrics,
            "evaluation_summary": self.evaluation_summary,
            "comparison": self.comparison,
            "recommendations": self.recommendations,
        }

        if self.report_json:
            try:
                with open(self.report_json, "w") as f:
                    json.dump(report_data, f, indent=2, default=str)
                print(f"\n✓ JSON report saved to: {self.report_json}")
            except Exception as e:
                print(f"\n✗ Failed to save JSON report: {e}")

        if self.report_md:
            try:
                md_content = self.generate_markdown_report(report_data)
                with open(self.report_md, "w") as f:
                    f.write(md_content)
                print(f"✓ Markdown report saved to: {self.report_md}")
            except Exception as e:
                print(f"✗ Failed to save Markdown report: {e}")

    def generate_markdown_report(self, report_data: Dict[str, Any]) -> str:
        """Generate a Markdown report."""
        lines = [
            "# CBO Demonstration Report",
            "",
            f"**Generated:** {report_data['timestamp']}",
            f"**Collection:** {report_data['collection_name']} (ID: {report_data['collection_id']})",
            f"**Number of Vectors:** {report_data['num_vectors']:,}",
            "",
            "## Configuration",
            "",
            f"- Vector Dimension: {report_data['config']['dim']}",
            f"- Index Type: {report_data['config']['index_type']}",
            f"- Metric Type: {report_data['config']['metric_type']}",
            f"- Index Params (nlist): {report_data['config']['nlist']}",
            f"- Search Params (nprobe): {report_data['config']['nprobe']}",
            f"- Top-K: {report_data['config']['topk']}",
            "",
            "## Summary",
            "",
        ]

        if self.evaluation_summary:
            eval_sum = self.evaluation_summary
            lines.extend([
                f"- Total Queries: {eval_sum.get('TotalQueries', 0):,}",
                f"- Queries with CBO: {eval_sum.get('QueriesWithCBO', 0):,}",
                f"- Queries Improved: {eval_sum.get('QueriesImproved', 0):,}",
                f"- Queries Degraded: {eval_sum.get('QueriesDegraded', 0):,}",
                f"- Queries Neutral: {eval_sum.get('QueriesNeutral', 0):,}",
                f"- Average Time Reduction: {eval_sum.get('AvgTimeReductionPercent', 0):.2f}%",
                "",
            ])

        if self.recommendations:
            lines.extend([
                "## Recommendations",
                "",
            ])
            for rec in self.recommendations:
                lines.append(f"- {rec}")
            lines.append("")

        return "\n".join(lines)

    def print_final_summary(self) -> None:
        """Print final summary with key information."""
        print("\n" + "=" * 80)
        print("Final Summary")
        print("=" * 80)
        print(f"Collection: {self.collection_name}")
        if self.collection_id:
            print(f"Collection ID: {self.collection_id}")
        if self.collection:
            print(f"Entities: {self.collection.num_entities:,}")
        print(f"Index: {self.index_type} ({self.metric_type}, nlist={self.nlist})")
        print(f"HTTP API: {self.http_base_url}")
        if self.report_json:
            print(f"JSON Report: {self.report_json}")
        if self.report_md:
            print(f"Markdown Report: {self.report_md}")
        print("=" * 80)

    def run(self, drop_existing: bool = True, skip_index: bool = False, skip_load: bool = False) -> bool:
        """
        Run the complete CBO demonstration.
        """
        self.print_config()

        # Step 1: Check Milvus readiness
        if not self.check_milvus_ready():
            print("\n✗ Milvus is not ready. Please ensure Milvus is running and accessible.")
            return False

        # Step 2: Setup collection
        if not self.setup_collection(drop_existing=drop_existing):
            return False

        # Step 3: Insert data
        if not self.insert_data():
            return False

        # Step 4: Create index and load
        if not self.create_index_and_load(skip_index=skip_index, skip_load=skip_load):
            return False

        # Step 5: Warmup queries
        self.execute_warmup_queries()

        # Step 6: Execute queries
        query_results = self.execute_queries()

        # Step 7: Display results
        self.display_results(query_results)

        return True


def main():
    parser = argparse.ArgumentParser(
        description="CBO Customization Demonstration",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Basic run with defaults
  python scripts/cbo_demo.py

  # Custom collection and vector count
  python scripts/cbo_demo.py --collection my_collection --num-vectors 200000

  # Generate reports
  python scripts/cbo_demo.py --report-json cbo_report.json --report-md cbo_report.md

  # Custom index parameters
  python scripts/cbo_demo.py --index-type IVF_SQ8 --nlist 256 --nprobe 20
        """,
    )
    parser.add_argument(
        "--host",
        type=str,
        default=DEFAULT_MILVUS_HOST,
        help=f"Milvus host (default: {DEFAULT_MILVUS_HOST})",
    )
    parser.add_argument(
        "--port",
        type=str,
        default=DEFAULT_MILVUS_PORT,
        help=f"Milvus port (default: {DEFAULT_MILVUS_PORT})",
    )
    parser.add_argument(
        "--http-port",
        type=int,
        default=DEFAULT_HTTP_PORT,
        help=f"Milvus HTTP port (default: {DEFAULT_HTTP_PORT})",
    )
    parser.add_argument(
        "--collection",
        type=str,
        default=DEFAULT_COLLECTION_NAME,
        help=f"Collection name (default: {DEFAULT_COLLECTION_NAME})",
    )
    parser.add_argument(
        "--num-vectors",
        type=int,
        default=DEFAULT_NUM_VECTORS,
        help=f"Number of vectors to insert (default: {DEFAULT_NUM_VECTORS:,})",
    )
    parser.add_argument(
        "--dim",
        type=int,
        default=DEFAULT_DIM,
        help=f"Vector dimension (default: {DEFAULT_DIM})",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=DEFAULT_BATCH_SIZE,
        help=f"Batch size for insertion (default: {DEFAULT_BATCH_SIZE:,})",
    )
    parser.add_argument(
        "--seed",
        type=int,
        default=DEFAULT_SEED,
        help=f"Random seed for reproducibility (default: {DEFAULT_SEED})",
    )
    parser.add_argument(
        "--index-type",
        type=str,
        default=DEFAULT_INDEX_TYPE,
        help=f"Index type (default: {DEFAULT_INDEX_TYPE})",
    )
    parser.add_argument(
        "--metric-type",
        type=str,
        default=DEFAULT_METRIC_TYPE,
        help=f"Metric type (default: {DEFAULT_METRIC_TYPE})",
    )
    parser.add_argument(
        "--nlist",
        type=int,
        default=DEFAULT_NLIST,
        help=f"Index parameter nlist (default: {DEFAULT_NLIST})",
    )
    parser.add_argument(
        "--nprobe",
        type=int,
        default=DEFAULT_NPROBE,
        help=f"Search parameter nprobe (default: {DEFAULT_NPROBE})",
    )
    parser.add_argument(
        "--topk",
        type=int,
        default=DEFAULT_TOPK,
        help=f"Top-K for search (default: {DEFAULT_TOPK})",
    )
    parser.add_argument(
        "--warmup-queries",
        type=int,
        default=DEFAULT_WARMUP_QUERIES,
        help=f"Number of warmup queries (default: {DEFAULT_WARMUP_QUERIES})",
    )
    parser.add_argument(
        "--report-json",
        type=str,
        default=None,
        help="Path to save JSON report (optional)",
    )
    parser.add_argument(
        "--report-md",
        type=str,
        default=None,
        help="Path to save Markdown report (optional)",
    )
    parser.add_argument(
        "--keep-collection",
        action="store_true",
        help="Keep existing collection if it exists (default: drop and recreate)",
    )
    parser.add_argument(
        "--no-index",
        action="store_true",
        help="Skip index creation (use existing index)",
    )
    parser.add_argument(
        "--no-load",
        action="store_true",
        help="Skip collection loading (assume already loaded)",
    )

    args = parser.parse_args()

    demo = CBODemo(
        milvus_host=args.host,
        milvus_port=args.port,
        http_port=args.http_port,
        collection_name=args.collection,
        num_vectors=args.num_vectors,
        dim=args.dim,
        batch_size=args.batch_size,
        seed=args.seed,
        index_type=args.index_type,
        metric_type=args.metric_type,
        nlist=args.nlist,
        nprobe=args.nprobe,
        topk=args.topk,
        warmup_queries=args.warmup_queries,
        report_json=args.report_json,
        report_md=args.report_md,
    )

    success = demo.run(
        drop_existing=not args.keep_collection,
        skip_index=args.no_index,
        skip_load=args.no_load,
    )
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
