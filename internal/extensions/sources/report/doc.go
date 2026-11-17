// SPDX-License-Identifier: Apache-2.0

// Package report is the neutral tabular report runner of the source kit: a
// report definition (variables, query and transform steps, a workbook output)
// run over a source's query function (QueryFunc), with the built-in
// transformers filter-rows, derive-column and group-by-relation. Its writers
// (csv, newline JSON, table, xlsx workbook) are in report/output.
package report
