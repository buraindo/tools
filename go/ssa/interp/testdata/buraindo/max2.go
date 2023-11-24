package main

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	println(max2(5, 31))
}
