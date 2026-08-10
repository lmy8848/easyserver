package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// cliQueryRunner runs SQL by exec'ing the engine CLI inside the managed
// container (docker/podman exec) and parsing the text output. It is the fallback
// channel for instances direct connection cannot reach: Redis (no driver in
// scope) and MySQL/PostgreSQL instances whose host port mapping is broken
// (container_port = 0). Text output loses types — every column reports "string"
// and NULL cannot be distinguished from the string "NULL" — which is exactly the
// gap the direct channel closes; this runner is kept for the instances that
// cannot use it.
type cliQueryRunner struct {
	exec func(ctx context.Context, inst *DBInstance, args ...string) (string, error) // Service.runInVersion
}

func (r *cliQueryRunner) Query(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*QueryResult, error) {
	rendered, err := renderSQL(inst.DBType, sql, args)
	if err != nil {
		return nil, err
	}
	var out string
	switch inst.DBType {
	case DBTypeMySQL:
		// -B batch mode: tab-separated with the header on the first line (no -N so
		// column names survive for the table browser). Auth warnings go to stderr,
		// which the separated exec keeps out of this parse stream.
		out, err = r.exec(ctx, inst, "mysql", dbName, "-B", "-e", rendered)
	case DBTypePostgreSQL:
		// -A unaligned: header on the first line, rows below the ---- separator,
		// "(N rows)" footer at the end — all skipped by parseCLIQueryResult.
		out, err = r.exec(ctx, inst, "psql", "-d", dbName, "-A", "-c", rendered)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", inst.DBType)
	}
	if err != nil {
		return nil, err
	}
	return parseCLIQueryResult(inst.DBType, out), nil
}

func (r *cliQueryRunner) Exec(ctx context.Context, inst *DBInstance, dbName, sql string, args ...any) (*ExecResult, error) {
	rendered, err := renderSQL(inst.DBType, sql, args)
	if err != nil {
		return nil, err
	}
	var out string
	switch inst.DBType {
	case DBTypeMySQL:
		// mysql -e prints a "Query OK, N row(s) affected" status line on stdout.
		out, err = r.exec(ctx, inst, "mysql", dbName, "-e", rendered)
		if err != nil {
			return nil, err
		}
		return &ExecResult{RowsAffected: parseMySQLAffected(out)}, nil
	case DBTypePostgreSQL:
		// psql -c prints a command tag ("INSERT 0 1", "UPDATE 2", "DELETE 3").
		out, err = r.exec(ctx, inst, "psql", "-d", dbName, "-c", rendered)
		if err != nil {
			return nil, err
		}
		return &ExecResult{RowsAffected: parsePGCommandTag(out)}, nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", inst.DBType)
	}
}

func (r *cliQueryRunner) Close(int64) {}

// parseCLIQueryResult parses the CLI text output into a structured result. mysql
// -B output is tab-separated (header on line 0); psql -A output is |-separated
// with a ---- separator line and a "(N rows)" footer to skip. All values are
// strings — the CLI channel cannot recover types.
func parseCLIQueryResult(dbType DBType, out string) *QueryResult {
	res := &QueryResult{}
	for i, rawLine := range strings.Split(out, "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		if i == 0 {
			for _, f := range splitCLIField(dbType, line) {
				res.Columns = append(res.Columns, ColumnMeta{Name: strings.TrimSpace(f), Type: "string"})
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if dbType == DBTypePostgreSQL {
			if strings.HasPrefix(trimmed, "(") || isSeparatorLine(trimmed) {
				continue
			}
		}
		fields := splitCLIField(dbType, line)
		row := make([]any, len(fields))
		for j, f := range fields {
			row[j] = strings.TrimSpace(f)
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

func splitCLIField(dbType DBType, line string) []string {
	if dbType == DBTypePostgreSQL {
		return strings.Split(line, "|")
	}
	return strings.Split(line, "\t")
}

// isSeparatorLine reports whether a psql -A line is the ---- divider. It must
// contain a dash and consist only of separator glyphs, so a data row like "-5"
// (negative number) is never mistaken for one.
func isSeparatorLine(t string) bool {
	if !strings.Contains(t, "-") {
		return false
	}
	for _, r := range t {
		switch r {
		case '-', '+', '|', ' ', '\t':
		default:
			return false
		}
	}
	return true
}

// parseMySQLAffected extracts the affected-row count from mysql -e's status line
// ("Query OK, 3 rows affected (0.00 sec)").
func parseMySQLAffected(out string) int64 {
	// The count follows "Query OK, " and precedes " row".
	start := strings.Index(out, "Query OK,")
	if start < 0 {
		return 0
	}
	rest := out[start+len("Query OK,"):]
	rest = strings.TrimSpace(rest)
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, _ := strconv.ParseInt(rest[:end], 10, 64)
	return n
}

// parsePGCommandTag extracts the affected-row count from a psql command tag
// ("INSERT 0 1", "UPDATE 2", "DELETE 3"). Tags without a trailing count (e.g.
// "CREATE TABLE") yield 0.
func parsePGCommandTag(out string) int64 {
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) == 0 {
		return 0
	}
	// The tag may be the whole line ("UPDATE 2") or just the last token.
	for i := len(fields) - 1; i >= 0; i-- {
		if n, err := strconv.ParseInt(fields[i], 10, 64); err == nil {
			return n
		}
	}
	return 0
}
