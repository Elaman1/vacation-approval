package middleware

import "testing"

func TestHasRole(t *testing.T) {
	if !hasRole([]string{"employee", "default-roles-vacation"}, "employee") {
		t.Fatal("employee role must match")
	}
	if hasRole([]string{"employee"}, "manager") {
		t.Fatal("employee must not have manager access")
	}
}
