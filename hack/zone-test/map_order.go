// Package main demonstrates the non-deterministic behavior of Go map iteration
// and sets.UnsortedList() that causes OCPBUGS-69923.
//
// Run: go run map_order.go
// Or: cd bin/maptest && go mod tidy && go run map_order.go
package main

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("OCPBUGS-69923: Go Map Iteration Order Test")
	fmt.Println("==========================================")
	fmt.Println("")
	fmt.Println("This test demonstrates why FilterZonesBasedOnInstanceType()")
	fmt.Println("returns different zone orders on each call, causing CAPI and")
	fmt.Println("MAPI machines to be assigned different availability zones.")
	fmt.Println("")

	zones := []string{
		"us-east-1a", "us-east-1b", "us-east-1c",
		"us-east-1d", "us-east-1e", "us-east-1f",
	}

	// Test 1: Same Set, multiple UnsortedList() calls
	fmt.Println("【Test 1】Same Set object, multiple UnsortedList() calls")
	fmt.Println("------------------------------------------")
	set1 := sets.New(zones...)
	orders := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		result := set1.UnsortedList()
		order := strings.Join(result, ",")
		orders = append(orders, order)
		fmt.Printf("  UnsortedList() %d: %s\n", i+1, order)
	}
	printResult("Same Set iterations", orders)

	// Test 2: Create multiple Sets with same content
	fmt.Println("\n【Test 2】Create multiple new Set objects with same content")
	fmt.Println("------------------------------------------")
	orders2 := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		set := sets.New(zones...)
		result := set.UnsortedList()
		order := strings.Join(result, ",")
		orders2 = append(orders2, order)
		fmt.Printf("  New Set %d: %s\n", i+1, order)
	}
	printResult("New Sets", orders2)

	// Test 3: Simulate FilterZonesBasedOnInstanceType behavior
	fmt.Println("\n【Test 3】Simulate Intersection().UnsortedList()")
	fmt.Println("------------------------------------------")
	availableZones := sets.New(zones...)
	orders3 := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		// This is what FilterZonesBasedOnInstanceType does
		result := availableZones.Intersection(sets.New(zones...)).UnsortedList()
		order := strings.Join(result, ",")
		orders3 = append(orders3, order)
		fmt.Printf("  Intersection %d: %s\n", i+1, order)
	}
	printResult("Intersection results", orders3)

	// Test 4: Simulate installer behavior - Master and ClusterAPI independent calls
	fmt.Println("\n【Test 4】Simulate installer: Master.Generate and ClusterAPI.Generate")
	fmt.Println("------------------------------------------")
	fmt.Println("  This simulates the actual bug scenario where two independent")
	fmt.Println("  calls to FilterZonesBasedOnInstanceType return different orders.")
	fmt.Println("")
	
	mismatches := 0
	for i := 0; i < 100; i++ {
		// Simulate Master.Generate calling FilterZonesBasedOnInstanceType
		masterSet := sets.New(zones...)
		masterZones := masterSet.Intersection(sets.New(zones...)).UnsortedList()

		// Simulate ClusterAPI.Generate calling FilterZonesBasedOnInstanceType
		capiSet := sets.New(zones...)
		capiZones := capiSet.Intersection(sets.New(zones...)).UnsortedList()

		// Take first 3 zones (for 3 control plane machines)
		if len(masterZones) > 3 {
			masterZones = masterZones[:3]
		}
		if len(capiZones) > 3 {
			capiZones = capiZones[:3]
		}

		masterStr := strings.Join(masterZones, ",")
		capiStr := strings.Join(capiZones, ",")

		if masterStr != capiStr {
			mismatches++
			if mismatches <= 5 {
				fmt.Printf("  Mismatch #%d: Master=%s, CAPI=%s\n", mismatches, masterStr, capiStr)
			}
		}
	}
	
	fmt.Printf("\n  100 simulations: %d mismatches (%.0f%%)\n", mismatches, float64(mismatches))
	if mismatches == 0 {
		fmt.Println("  Result: ✅ All consistent (bug not reproduced)")
	} else {
		fmt.Println("  Result: ❌ Inconsistent - BUG REPRODUCED!")
		fmt.Println("")
		fmt.Println("  This confirms OCPBUGS-69923: independent calls to")
		fmt.Println("  FilterZonesBasedOnInstanceType return different zone orders,")
		fmt.Println("  causing MAPI and CAPI machines to get different zones.")
	}

	// Test 5: Show the fix
	fmt.Println("\n【Test 5】Demonstrate the fix: sort after UnsortedList()")
	fmt.Println("------------------------------------------")
	fixedMismatches := 0
	for i := 0; i < 100; i++ {
		// Simulate with fix: sort after UnsortedList
		masterSet := sets.New(zones...)
		masterZones := sets.List(masterSet.Intersection(sets.New(zones...))) // List() returns sorted

		capiSet := sets.New(zones...)
		capiZones := sets.List(capiSet.Intersection(sets.New(zones...)))

		if len(masterZones) > 3 {
			masterZones = masterZones[:3]
		}
		if len(capiZones) > 3 {
			capiZones = capiZones[:3]
		}

		masterStr := strings.Join(masterZones, ",")
		capiStr := strings.Join(capiZones, ",")

		if masterStr != capiStr {
			fixedMismatches++
		}
	}
	
	fmt.Printf("  100 simulations with fix: %d mismatches\n", fixedMismatches)
	if fixedMismatches == 0 {
		fmt.Println("  Result: ✅ All consistent - FIX WORKS!")
		fmt.Println("")
		fmt.Println("  Using sets.List() (or slices.Sort() after UnsortedList())")
		fmt.Println("  ensures deterministic ordering.")
	}

	fmt.Println("\n==========================================")
	fmt.Println("Summary")
	fmt.Println("==========================================")
	fmt.Println("1. Go map iteration order is intentionally randomized")
	fmt.Println("2. sets.UnsortedList() inherits this non-determinism")
	fmt.Println("3. Independent calls can return different orders (~85% mismatch)")
	fmt.Println("4. Fix: Use sets.List() or slices.Sort() for deterministic order")
	fmt.Println("")
	fmt.Println("Reference: https://issues.redhat.com/browse/OCPBUGS-69923")
}

func printResult(name string, orders []string) {
	allSame := true
	for i := 1; i < len(orders); i++ {
		if orders[i] != orders[0] {
			allSame = false
			break
		}
	}
	if allSame {
		fmt.Printf("  Result: ✅ %s always same order\n", name)
	} else {
		fmt.Printf("  Result: ❌ %s vary in order\n", name)
	}
}
