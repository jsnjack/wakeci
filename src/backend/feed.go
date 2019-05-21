package main

// MaxFeedList maximum amount of tasks that are being executed at the same time
const MaxFeedList = 2

// FeedList contains all tasks that are being executed at the moment
var FeedList []*Executor

// FeedQueue ...
var FeedQueue []*Executor
