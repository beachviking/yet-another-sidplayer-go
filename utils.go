package main

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// // func absInt(x int) int {
// // 	return absDiffInt(x, 0)
// // }

// // func absDiffInt(x, y int) int {
// // 	if x < y {
// // 		return y - x
// // 	}
// // 	return x - y
// // }
