package slicex

import "testing"

func TestContains(t *testing.T) {
	slice := []string{"_pending", "_to_be_paid", "_confirmed", "_to_be_shipped", "_shipping"}

	// Test case 1: All values present in the slice
	result := Contains(slice, "_to_be_paid", "_confirmed", "_shipping")
	if !result {
		t.Errorf("Test case 1 failed. Expected: true, got: false")
	}

	// Test case 2: Some values are missing from the slice
	result = Contains(slice, "_to_be_paid", "_shipped")
	if result {
		t.Errorf("Test case 2 failed. Expected: false, got: true")
	}

	// Test case 3: Empty slice and empty values
	emptySlice := []string{}
	result = Contains(emptySlice)
	if !result {
		t.Errorf("Test case 3 failed. Expected: true, got: false")
	}

	// Test case 4: Empty slice but with some values
	result = Contains(emptySlice, "_to_be_paid")
	if result {
		t.Errorf("Test case 4 failed. Expected: false, got: true")
	}
}
