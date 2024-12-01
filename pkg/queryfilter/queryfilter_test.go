package queryfilter

import (
	"testing"
)

func TestParseQueryString(t *testing.T) {
	tests := []struct {
		name            string
		queryString     string
		expectedFilters []Filter
		expectError     bool
	}{
		{
			name:        "Simple equals",
			queryString: "age=5",
			expectedFilters: []Filter{
				{Field: "age", Operator: Eq, Value: "5"},
			},
		},
		{
			name:        "Greater than",
			queryString: "age[gt]=5",
			expectedFilters: []Filter{
				{Field: "age", Operator: Gt, Value: "5"},
			},
		},
		{
			name:        "Less than or equal",
			queryString: "age[lte]=5",
			expectedFilters: []Filter{
				{Field: "age", Operator: Lte, Value: "5"},
			},
		},
		{
			name:        "JSON format",
			queryString: `age={"lt":100}&name={"contains":"m"}`,
			expectedFilters: []Filter{
				{Field: "age", Operator: Lt, Value: float64(100)},
				{Field: "name", Operator: Contains, Value: "m"},
			},
		},
		{
			name:        "Multiple filters",
			queryString: "age[gte]=18&name[sw]=J&active=true",
			expectedFilters: []Filter{
				{Field: "age", Operator: Gte, Value: "18"},
				{Field: "name", Operator: StartsWith, Value: "J"},
				{Field: "active", Operator: Eq, Value: "true"},
			},
		},
		{
			name:        "Case insensitive operators",
			queryString: "age[GT]=5&name[Contains]=John",
			expectedFilters: []Filter{
				{Field: "age", Operator: Gt, Value: "5"},
				{Field: "name", Operator: Contains, Value: "John"},
			},
		},
		{
			name:        "Invalid JSON",
			queryString: `age={"lt":100&name={"contains":"m"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseQueryString(tt.queryString)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			for _, filter := range tt.expectedFilters {
				if len(filters) != len(tt.expectedFilters) {
					t.Errorf("Filters do not match expected.\nGot: %#v\nWant: %#v", filters, tt.expectedFilters)
				}

				found := false
				for _, f := range filters {
					if f.Field == filter.Field && f.Operator == filter.Operator && f.Value == filter.Value {
						found = true
						break
					}
				}

				if !found {
					t.Errorf("Filter not found: %#v", filter)
				}
			}
		})
	}
}

func TestMapOperator(t *testing.T) {
	tests := []struct {
		input    string
		expected Operator
	}{
		{"gt", Gt},
		{"GT", Gt},
		{"gte", Gte},
		{"GTE", Gte},
		{"lt", Lt},
		{"LT", Lt},
		{"lte", Lte},
		{"LTE", Lte},
		{"ne", Ne},
		{"NE", Ne},
		{"sw", StartsWith},
		{"SW", StartsWith},
		{"ew", EndsWith},
		{"EW", EndsWith},
		{"contains", Contains},
		{"includes", Contains},
		{"CONTAINS", Contains},
		{"notContains", DoesNotContain},
		{"NOTCONTAINS", DoesNotContain},
		{"startsWith", StartsWith},
		{"STARTSWITH", StartsWith},
		{"endsWith", EndsWith},
		{"ENDSWITH", EndsWith},
		{"notStartsWith", DoesNotStartWith},
		{"NOTSTARTSWITH", DoesNotStartWith},
		{"notEndsWith", DoesNotEndWith},
		{"NOTENDSWITH", DoesNotEndWith},
		{"between", Between},
		{"BETWEEN", Between},
		{"before", Before},
		{"BEFORE", Before},
		{"after", After},
		{"AFTER", After},
		{"unknown", Eq},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := MapOperator(tt.input)
			if result != tt.expected {
				t.Errorf("For input %s, expected %v, but got %v", tt.input, tt.expected, result)
			}
		})
	}
}
