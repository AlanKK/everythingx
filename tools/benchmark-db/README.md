# SQLite Performance Benchmark Results

This README presents the results of performance benchmarks conducted on SQLite queries with various optimizations and configurations. The benchmarks were run on February 9, 2025, to evaluate the impact of different indexing strategies and SQLite PRAGMA settings on query performance.

## Benchmark Setup

- **Query**: `SELECT fullpath, object_type FROM files WHERE filename LIKE ? COLLATE BINARY ORDER BY filename ASC LIMIT ?`
- **Table Structure**:
  ```sql
  CREATE TABLE files(
    filename TEXT NOT NULL,
    fullpath TEXT NOT NULL UNIQUE,
    event_id INTEGER,
    object_type INTEGER
  );
  ```

## Baseline Performance (No Index)

Initial tests were run without any index and with WAL journal mode and synchronous mode off.

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:08:26 | Overall | 104 | 142 | 111     | 109    | 6      | 400 iterations  |
| 2025/02/09 10:09:12 | Overall | 106 | 201 | 114     | 111    | 9      | 400 iterations  |
| 2025/02/09 10:09:59 | Overall | 108 | 173 | 117     | 114    | 9      | 400 iterations  |

**Key Insight**: Without an index, query performance is consistently slow, with average times above 110ms.

## Performance with Basic Index

A basic index was added on the filename column:

```sql
CREATE INDEX idx_filename ON files(filename COLLATE BINARY);
```

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:12:19 | Overall | 13  | 111 | 55      | 68     | 24     | 400 iterations  |
| 2025/02/09 10:12:41 | Overall | 14  | 76  | 55      | 67     | 23     | 400 iterations  |
| 2025/02/09 10:13:04 | Overall | 14  | 138 | 56      | 68     | 24     | 400 iterations  |

**Key Insight**: Adding a basic index on the filename column dramatically improved performance, reducing average query time by about 50%.

## Extended Testing with Basic Index

Further tests were conducted with the basic index and increased iterations:

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:17:28 | Overall | 13  | 608 | 61      | 68     | 41     | 1200 iterations |
| 2025/02/09 10:18:35 | Overall | 14  | 83  | 55      | 68     | 23     | 1200 iterations |
| 2025/02/09 10:19:43 | Overall | 14  | 122 | 56      | 67     | 24     | 1200 iterations |

**Key Insight**: Increasing the number of iterations to 1200 showed consistent performance, with occasional spikes in maximum query time.

## Composite Index Performance

A composite index was tested to see if it would improve performance:

```sql
CREATE INDEX idx_filename_fullpath_obj_type ON files(filename COLLATE BINARY, fullpath, object_type);
```

| Date                | Search  | Min | Max  | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|------|---------|--------|--------|-----------------|
| 2025/02/09 10:28:47 | Overall | 16  | 1093 | 100     | 113    | 0      | 1200 iterations |
| 2025/02/09 10:30:35 | Overall | 16  | 188  | 90      | 114    | 42     | 1200 iterations |
| 2025/02/09 10:32:47 | Overall | 16  | 2669 | 109     | 116    | 0      | 1200 iterations |

**Key Insight**: The composite index actually degraded performance, likely due to the query optimizer choosing this more complex index over the simpler, more efficient one.

## PRAGMA Settings Impact

Various PRAGMA settings were tested to assess their impact on read query performance:

### Without WAL and Synchronous OFF

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:44:20 | Overall | 13  | 346 | 55      | 67     | 25     | 1200 iterations |
| 2025/02/09 10:45:27 | Overall | 14  | 79  | 55      | 67     | 23     | 1200 iterations |
| 2025/02/09 10:46:34 | Overall | 14  | 133 | 55      | 67     | 23     | 1200 iterations |

### With PRAGMA journal_mode=WAL

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:49:11 | Overall | 13  | 147 | 55      | 67     | 24     | 1200 iterations |
| 2025/02/09 10:50:18 | Overall | 14  | 84  | 55      | 67     | 23     | 1200 iterations |
| 2025/02/09 10:51:27 | Overall | 14  | 111 | 56      | 68     | 24     | 1200 iterations |

### With PRAGMA synchronous=OFF

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:54:42 | Overall | 13  | 79  | 56      | 68     | 24     | 1200 iterations |
| 2025/02/09 10:55:51 | Overall | 14  | 128 | 56      | 69     | 24     | 1200 iterations |
| 2025/02/09 10:56:58 | Overall | 14  | 78  | 55      | 68     | 23     | 1200 iterations |

### With PRAGMA synchronous=NORMAL

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:59:46 | Overall | 13  | 300 | 56      | 68     | 25     | 1200 iterations |
| 2025/02/09 11:00:53 | Overall | 14  | 129 | 56      | 68     | 24     | 1200 iterations |
| 2025/02/09 11:02:01 | Overall | 14  | 122 | 56      | 68     | 24     | 1200 iterations |

**Key Insight**: For read-only queries, PRAGMA settings had minimal impact on performance. The synchronous=NORMAL setting was kept for a balance of performance and data integrity.

## Index Collation Impact

Tests were conducted to evaluate the impact of COLLATE BINARY in the index and query:

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 11:06:42 | Overall | 13  | 112 | 56      | 68     | 24     | 1200 iterations |
| 2025/02/09 11:07:50 | Overall | 14  | 105 | 56      | 68     | 23     | 1200 iterations |
| 2025/02/09 11:08:59 | Overall | 14  | 530 | 57      | 68     | 28     | 1200 iterations |

**Key Insight**: Including COLLATE BINARY in both the index and query provided consistent performance.

## Result Limit Impact

Tests with different maxSearchResults values:

### maxSearchResults = 2000

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 11:12:30 | Overall | 10  | 927 | 49      | 68     | 40     | 1200 iterations |
| 2025/02/09 11:13:27 | Overall | 10  | 140 | 47      | 66     | 24     | 1200 iterations |
| 2025/02/09 11:14:23 | Overall | 10  | 93  | 46      | 61     | 24     | 1200 iterations |

### maxSearchResults = 1000

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 11:56:22 | Overall | 9   | 573 | 46      | 67     | 31     | 1200 iterations |
| 2025/02/09 11:57:17 | Overall | 10  | 83  | 45      | 57     | 24     | 1200 iterations |
| 2025/02/09 11:58:13 | Overall | 10  | 112 | 46      | 65     | 25     | 1200 iterations |

**Key Insight**: Reducing maxSearchResults from 2000 to 1000 slightly improved average and median query times.

## Final Optimization: Wildcard Placement

A significant improvement was achieved by changing the wildcard placement in the query:

```sql
prefixSearchStmt.Query("%"+prefix+"%", resultLimit)
```

| Date                | Search  | Min | Max | Average | Median | StdDev | Notes           |
|---------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 13:23:58 | Overall | 1   | 122 | 34      | 19     | 42     | 1200 iterations |
| 2025/02/09 13:24:39 | Overall | 1   | 134 | 34      | 14     | 42     | 1200 iterations |
| 2025/02/09 13:25:20 | Overall | 1   | 152 | 34      | 15     | 42     | 1200 iterations |

**Key Insight**: Placing wildcards at both ends of the search term dramatically improved performance, reducing median query times to under 20ms.

## Conclusion

The most significant performance improvements were achieved by:
1. Adding a basic index on the filename column
2. Optimizing the wildcard placement in the query

PRAGMA settings and result limits had minimal impact on read-only query performance. The composite index unexpectedly degraded performance and should be avoided for this specific query pattern.

Citations:
[1] https://ppl-ai-file-upload.s3.amazonaws.com/web/direct-files/27640979/08fe1c36-e642-4a4c-896e-dc47e8244c34/benchmark_results.csv

---
Answer from Perplexity: pplx.ai/share