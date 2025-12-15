#!/usr/bin/env python3
"""
CBO Demonstration Script
Tests the Cost-Based Optimizer with different filter selectivities
"""

from pymilvus import connections, Collection, FieldSchema, CollectionSchema, DataType
import numpy as np
import random
import time
from datetime import datetime
import subprocess

def find_cbo_log_file(container="milvus-builder-1"):
    """Find the CBO log file location"""
    # Possible CBO log file locations (in order of likelihood)
    possible_paths = [
        "/go/src/github.com/milvus-io/milvus/logs/cbo.log",
        "/tmp/milvus/logs/cbo.log",
        "/tmp/cbo.log",
        "logs/cbo.log",  # Relative path
    ]
    
    for log_path in possible_paths:
        try:
            # Check if file exists
            cmd = f"docker exec {container} test -f {log_path} && echo {log_path}"
            result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=3)
            if result.returncode == 0 and result.stdout.strip():
                return result.stdout.strip()
        except Exception:
            continue
    
    return None

def search_milvus_logs(pattern="CBO Decision", container="milvus-builder-1", lines=20):
    """Helper function to search Milvus logs for CBO decisions
    Searches both the dedicated CBO log file and the main standalone.log
    """
    found_any = False
    
    # First, try to find and search the CBO log file
    cbo_log_file = find_cbo_log_file(container)
    if cbo_log_file:
        print(f"📁 Found CBO log file: {cbo_log_file}")
        try:
            cmd = f"docker exec {container} grep -i '{pattern}' {cbo_log_file} | tail -{lines}"
            result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=5)
            if result.returncode == 0 and result.stdout.strip():
                print(f"\n📋 Found CBO logs in dedicated file matching '{pattern}':")
                print("-" * 60)
                print(result.stdout)
                print("-" * 60)
                found_any = True
        except Exception as e:
            print(f"\n⚠️  Could not search CBO log file: {e}")
    else:
        print("⚠️  CBO log file not found, searching main log file...")
    
    # Also search the main standalone.log (CBO logs are written to both)
    try:
        cmd = f"docker exec {container} grep -i '{pattern}' /tmp/standalone.log | tail -{lines}"
        result = subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=5)
        if result.returncode == 0 and result.stdout.strip():
            if not found_any:
                print(f"\n📋 Found CBO logs in main log file matching '{pattern}':")
                print("-" * 60)
            else:
                print(f"\n📋 Also found in main log file:")
                print("-" * 60)
            print(result.stdout)
            print("-" * 60)
            found_any = True
    except Exception as e:
        if not found_any:
            print(f"\n⚠️  Could not search main log file: {e}")
    
    if not found_any:
        print(f"\n⚠️  No logs found matching '{pattern}'")
        if cbo_log_file:
            print(f"   Searched: {cbo_log_file} and /tmp/standalone.log")
        else:
            print(f"   Searched: /tmp/standalone.log (CBO log file not found)")
    
    return found_any

# Connect
connections.connect("default", host="127.0.0.1", port="19530")

# Create collection
fields = [
    FieldSchema(name="id", dtype=DataType.INT64, is_primary=True),
    FieldSchema(name="price", dtype=DataType.DOUBLE),
    FieldSchema(name="category", dtype=DataType.INT64),
    FieldSchema(name="vector", dtype=DataType.FLOAT_VECTOR, dim=128)
]

schema = CollectionSchema(fields, "CBO Demo")
collection = Collection("cbo_demo", schema)

# Create index and load
collection.create_index("vector", {"index_type": "IVF_FLAT", "metric_type": "L2", "params": {"nlist": 1024}})
collection.load()

# Insert data
data = [
    [i for i in range(10000)],
    [random.uniform(0, 1000) for _ in range(10000)],
    [random.randint(1, 10) for _ in range(10000)],
    np.random.random((10000, 128)).tolist()
]
collection.insert(data)
collection.flush()

print("=" * 60)
print("CBO Demonstration")
print("=" * 60)

# Test 1: High selectivity (pre-filtering)
print("\n" + "🔥" * 30)
print("[Test 1] High Selectivity Filter (price < 10)")
print("Expected: Standard filtering (selectivity < 0.05)")
print(f"⏰ Timestamp: {datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')}")
print("🔍 SEARCH MARKER: CBO_TEST_1_HIGH_SELECTIVITY_PRICE_LT_10")
print("🔥" * 30)
time.sleep(0.5)  # Small delay to make logs easier to catch

results = collection.search(
    data=[np.random.random((1, 128)).tolist()[0]],
    anns_field="vector",
    param={"metric_type": "L2", "params": {"nprobe": 10}},
    limit=10,
    expr="price < 10",
    output_fields=["price"]
)
print(f"Found {len(results[0])} results")
print("✅ Check Milvus logs for: '🔥🔥🔥 CBO Decision: Standard filtering selected 🔥🔥🔥'")
print("   Look for marker: CBO_TEST_1_HIGH_SELECTIVITY_PRICE_LT_10")
time.sleep(2)  # Delay to allow logs to be written
print("🔍 Searching Milvus logs...")
search_milvus_logs("CBO Decision.*Standard", lines=5)
time.sleep(1)  # Delay before next test

# Test 2: Low selectivity (post-filtering)
print("\n" + "🔥" * 30)
print("[Test 2] Low Selectivity Filter (category == 1)")
print("Expected: Iterative filtering (selectivity >= 0.05)")
print(f"⏰ Timestamp: {datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')}")
print("🔍 SEARCH MARKER: CBO_TEST_2_LOW_SELECTIVITY_CATEGORY_EQ_1")
print("🔥" * 30)
time.sleep(0.5)  # Small delay to make logs easier to catch

results = collection.search(
    data=[np.random.random((1, 128)).tolist()[0]],
    anns_field="vector",
    param={"metric_type": "L2", "params": {"nprobe": 10}},
    limit=10,
    expr="category == 1",
    output_fields=["category"]
)
print(f"Found {len(results[0])} results")
print("✅ Check Milvus logs for: '🔥🔥🔥 CBO Decision: Iterative filtering selected 🔥🔥🔥'")
print("   Look for marker: CBO_TEST_2_LOW_SELECTIVITY_CATEGORY_EQ_1")
time.sleep(2)  # Delay to allow logs to be written
print("🔍 Searching Milvus logs...")
search_milvus_logs("CBO Decision.*Iterative", lines=5)
time.sleep(1)  # Delay before summary

print("\n" + "=" * 60)
print("📋 FINAL CBO LOG SEARCH:")
print("=" * 60)
print("Searching for all CBO decisions...")
# Search for pattern that works in both CBO log file (no emojis) and main log (with emojis)
search_milvus_logs("CBO Decision", lines=10)
print("\n" + "=" * 60)
print("📋 MANUAL LOG SEARCH COMMANDS:")
print("=" * 60)

# Find CBO log file location
cbo_log_file = find_cbo_log_file()
if cbo_log_file:
    print(f"📁 CBO log file location: {cbo_log_file}")
    print("\nTo search the dedicated CBO log file:")
    print(f"  docker exec milvus-builder-1 grep -i 'CBO Decision' {cbo_log_file} | tail -20")
    print("\nOr view the entire CBO log file:")
    print(f"  docker exec milvus-builder-1 tail -50 {cbo_log_file}")
else:
    print("⚠️  CBO log file not found. Trying common locations:")
    print("  docker exec milvus-builder-1 find /go/src/github.com/milvus-io/milvus -name 'cbo.log' 2>/dev/null")
    print("  docker exec milvus-builder-1 find /tmp -name 'cbo.log' 2>/dev/null")

print("\nTo search the main log file (CBO logs are also written there):")
print("  docker exec milvus-builder-1 grep -i '🔥🔥🔥 CBO Decision' /tmp/standalone.log | tail -20")
print("\nOr search for all CBO logs in main file:")
print("  docker exec milvus-builder-1 grep -i 'CBO Decision' /tmp/standalone.log | tail -20")
print("\nOr search around current timestamp:")
print(f"  docker exec milvus-builder-1 grep '{datetime.now().strftime('%H:%M')}' /tmp/standalone.log | grep -i CBO")
print("=" * 60)