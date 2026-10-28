package postgres

import (
	"strings"
)

func classifySQL(sql string) (operation, collection string) {
	fields := strings.Fields(sql)
	if len(fields) == 0 {
		return "UNKNOWN", "unknown"
	}

	operation = strings.ToUpper(fields[0])
	collection = "unknown"

	switch operation {
	case "INSERT":
		if len(fields) >= 3 && strings.EqualFold(fields[1], "INTO") {
			collection = fields[2]
		}

	case "UPDATE":
		if len(fields) >= 2 {
			collection = fields[1]
		}

	case "DELETE":
		for i := 1; i+1 < len(fields); i++ {
			if strings.EqualFold(fields[i], "FROM") {
				collection = fields[i+1]
				break
			}
		}

	case "SELECT":
		for i := 1; i+1 < len(fields); i++ {
			if strings.EqualFold(fields[i], "FROM") {
				collection = fields[i+1]
				break
			}
		}
	}

	collection = strings.Trim(collection, `"'();`)

	return operation, collection
}
