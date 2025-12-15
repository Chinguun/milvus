# Query Processing Demo

This directory contains a demonstration script for testing Milvus Cost-Based Optimizer (CBO) with different filter selectivities.

## Setup

### Using Conda (Recommended)

Create and activate the conda environment:

```bash
conda env create -f environment.yml
conda activate query_processing_demo
```

### Using pip

Alternatively, you can use pip:

```bash
pip install -r requirements.txt
```

## Prerequisites

- Milvus server running on `127.0.0.1:19530`
- Conda or Python 3.9+ installed

## Running the Demo

```bash
python demo_script.py
```

The script will:
1. Create a collection with vector and scalar fields
2. Insert 10,000 sample records
3. Test high selectivity filter (pre-filtering expected)
4. Test low selectivity filter (post-filtering expected)

Check Milvus proxy logs to see the CBO decisions.

