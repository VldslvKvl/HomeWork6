package main

import (
	"math/big"
	"slices"
	"strconv"
)

type ListNode struct {
	Val  int
	Next *ListNode
}

func addTwoNumbers(l1 *ListNode, l2 *ListNode) *ListNode {
	reverseList := []int{}
	l1Sum := circListJoin(l1, reverseList)
	l2Sum := circListJoin(l2, reverseList)
	revertArray(l1Sum, l2Sum)
	num1 := arrToInt(l1Sum)
	num2 := arrToInt(l2Sum)
	sum := new(big.Int).Add(num1, num2)
	res := intToArr(sum)
	revertArray(res)
	return reverseListFinal(res)
}
func revertArray(arr ...[]int) {
	for _, nonSArr := range arr {
		slices.Reverse(nonSArr)
	}
}
func arrToInt(arr []int) *big.Int {
	strNum := new(big.Int)
	ten := big.NewInt(10)
	for _, digit := range arr {
		strNum.Mul(strNum, ten)
		strNum.Add(strNum, big.NewInt(int64(digit)))
	}
	return strNum
}
func intToArr(i *big.Int) []int {
	var res []int
	strNum := i.String()
	for _, v := range strNum {
		n, err := strconv.Atoi(string(v))
		if err != nil {
			panic(err)
		}
		res = append(res, n)
	}
	return res
}
func circListJoin(l1 *ListNode, reverseList []int) []int {
	if l1.Next != nil {
		val := circListJoin(l1.Next, reverseList)
		reverseList = append(reverseList, l1.Val)
		reverseList = append(reverseList, val...)
		return reverseList
	}
	reverseList = append(reverseList, l1.Val)
	return reverseList
}
func reverseListFinal(arr []int) *ListNode {
	var finalArr ListNode
	for i, n := range arr {
		if i != len(arr)-1 {
			finalArr.Next = reverseListFinal(arr[i+1:])
			finalArr.Val = n
			return &finalArr
		} else if i == len(arr)-1 {
			finalArr.Val = n
			finalArr.Next = nil
		}
	}
	return &finalArr
}

/*func main() {
	l1 := &ListNode{1, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{0, &ListNode{1, nil}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}
	l2 := &ListNode{5, &ListNode{6, &ListNode{4, nil}}}
	r := addTwoNumbers(l1, l2)
	fmt.Println(r)
}*/
