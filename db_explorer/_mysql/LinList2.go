package main

import (
	"fmt"
	"strconv"
	"strings"
)

type BrowserHistory struct {
	current *historyPage
}

type historyPage struct {
	prevUrl *historyPage
	url     string
	nextUrl *historyPage
}

func Constructor(homepage string) BrowserHistory {
	return BrowserHistory{current: &historyPage{url: homepage}}
}

func (this *BrowserHistory) Visit(url string) {
	page := &historyPage{prevUrl: this.current, url: url}
	// Новая страница заменяет всю историю переходов вперёд.
	this.current.nextUrl = page
	this.current = page
}

func (this *BrowserHistory) Back(steps int) string {
	for steps > 0 && this.current.prevUrl != nil {
		this.current = this.current.prevUrl
		steps--
	}
	return this.current.url
}

func (this *BrowserHistory) Forward(steps int) string {
	for steps > 0 && this.current.nextUrl != nil {
		this.current = this.current.nextUrl
		steps--
	}
	return this.current.url
}

type VersinStruct struct {
	currentV []string
}

func versComp(v1 *VersinStruct, v2 *VersinStruct) int {
	if len(v1.currentV) == len(v2.currentV) && len(v2.currentV) == 0 {
		return 0
	}
	if len(v1.currentV) == 0 {
		v1.currentV = append(v1.currentV, "0")
	}
	if len(v2.currentV) == 0 {
		v2.currentV = append(v2.currentV, "0")
	}
	num1, _ := strconv.Atoi(v1.currentV[0])
	num2, _ := strconv.Atoi(v2.currentV[0])
	for {
		if num1 > num2 {
			return 1
		} else if num1 < num2 {
			return -1
		} else {
			v1.currentV = v1.currentV[1:]
			v2.currentV = v2.currentV[1:]
			res := versComp(v1, v2)
			return res
		}

	}
}
func compareVersion(version1 string, version2 string) int {
	str1 := VersinStruct{strings.Split(version1, ".")}
	str2 := VersinStruct{strings.Split(version2, ".")}
	return versComp(&str1, &str2)
}

func main() {
	r := compareVersion("1.01", "1.001")
	fmt.Println(r)
}

/*func main() {
	sQ := []string{"visit", "visit", "visit", "back", "back", "forward", "visit", "forward", "back", "back"}
	s2 := []string{"google.com", "facebook.com", "youtube.com", "1", "1", "1", "linkedin.com", "2", "2", "7"}
	obj := Constructor("leetcode.com")
	for i, v := range sQ {
		switch v {
		case "visit":
			obj.Visit(s2[i])
		case "back":
			num, _ := strconv.Atoi(s2[i])
			fmt.Println(obj.Back(num))
		case "forward":
			num, _ := strconv.Atoi(s2[i])
			fmt.Println(obj.Forward(num))
		}
	}
}*/
