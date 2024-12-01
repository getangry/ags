package queryfilter

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Operator string

const (
	Eq               Operator = "="
	Ne               Operator = "!="
	Gt               Operator = ">"
	Gte              Operator = ">="
	Lt               Operator = "<"
	Lte              Operator = "<="
	Like             Operator = "LIKE"
	ILike            Operator = "ILIKE"
	Contains         Operator = "contains"
	Includes         Operator = "includes"
	DoesNotContain   Operator = "doesNotContain"
	StartsWith       Operator = "startsWith"
	EndsWith         Operator = "endsWith"
	DoesNotStartWith Operator = "doesNotStartWith"
	DoesNotEndWith   Operator = "doesNotEndWith"
	Between          Operator = "between"
	Before           Operator = "before"
	After            Operator = "after"
)

// Filter represents a query filter with a field, an operator, and a value.
// It is used to specify conditions for querying data.
type Filter struct {
	Field    string
	Operator Operator
	Value    interface{}
}

// MapOperator maps a string representation of an operator to its corresponding
// Operator type. The function is case-insensitive and supports the following
// operators:
// - "gt": Greater than
// - "gte": Greater than or equal to
// - "lt": Less than
// - "lte": Less than or equal to
// - "ne": Not equal to
// - "sw", "startswith": Starts with
// - "ew", "endswith": Ends with
// - "contains": Contains
// - "notcontains": Does not contain
// - "notstartswith": Does not start with
// - "notendswith": Does not end with
// - "between": Between
// - "before": Before
// - "after": After
// If the input string does not match any of the supported operators, the function
// returns the default operator "Eq" (equal).
func MapOperator(op string) Operator {
	switch strings.ToLower(op) {
	case "gt":
		return Gt
	case "gte":
		return Gte
	case "lt":
		return Lt
	case "lte":
		return Lte
	case "ne":
		return Ne
	case "sw", "startswith":
		return StartsWith
	case "ew", "endswith":
		return EndsWith
	case "includes", "contains":
		return Contains
	case "notcontains":
		return DoesNotContain
	case "notstartswith":
		return DoesNotStartWith
	case "notendswith":
		return DoesNotEndWith
	case "between":
		return Between
	case "before":
		return Before
	case "after":
		return After
	default:
		return Eq
	}
}

// ParseQueryString parses a query string into a slice of Filter objects.
// It supports three patterns for filters:
// 1. Field-based filters with operators, e.g., "age[gt]=30".
// 2. JSON-based filters, e.g., "filter={\"gt\":30}".
// 3. Simple equality filters, e.g., "name=John".
//
// Parameters:
//   - queryString: The query string to parse.
//
// Returns:
//   - A slice of Filter objects representing the parsed filters.
//   - An error if the query string is invalid or cannot be parsed.
func ParseQueryString(queryString string) ([]Filter, error) {
	values, err := url.ParseQuery(queryString)
	if err != nil {
		return nil, err
	}

	var filters []Filter

	for key, vals := range values {
		for _, val := range vals {
			if strings.Contains(key, "[") && strings.Contains(key, "]") {
				// First pattern
				field := strings.Split(key, "[")[0]
				op := strings.TrimSuffix(strings.Split(key, "[")[1], "]")
				filters = append(filters, Filter{Field: field, Operator: MapOperator(op), Value: val})
			} else if strings.HasPrefix(val, "{") {
				if strings.HasSuffix(val, "}") {
					// Second pattern (JSON)
					var jsonFilter map[string]interface{}

					err := json.Unmarshal([]byte(val), &jsonFilter)
					if err != nil {
						return nil, err
					}
					for op, value := range jsonFilter {
						filters = append(filters, Filter{Field: key, Operator: MapOperator(op), Value: value})
					}
				} else {
					return nil, fmt.Errorf("invalid JSON filter: %s", val)
				}
			} else {
				// Simple equals
				filters = append(filters, Filter{Field: key, Operator: Eq, Value: val})
			}
		}
	}

	return filters, nil
}

// ParseQueryFilters parses the query string from the request and returns a slice of Filters.
func ParseQueryFilters(r *http.Request) ([]Filter, error) {
	return ParseQueryString(r.URL.RawQuery)
}
