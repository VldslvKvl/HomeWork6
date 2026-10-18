package main

import (
	"fmt"
	"strconv"
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

func main() {
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
}
