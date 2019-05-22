package main

// MaxFeedList maximum amount of tasks that are being executed at the same time
const MaxFeedList = 2

// FeedList contains all tasks that are being executed at the moment
var FeedList []*Executor

// FeedQueue ...
var FeedQueue []*Executor

// TakeFromQueue checks if it is possible to start executing new job from queue
// and executes it
func TakeFromQueue() {
	if len(FeedList) < MaxFeedList && len(FeedQueue) > 0 {
		Logger.Printf("Taking job from queue %s\n", FeedQueue[0].ID)
		FeedList = append(FeedList, FeedQueue[0])
		go FeedQueue[0].Start()
		FeedQueue[0] = nil
		FeedQueue = FeedQueue[1:]
		TakeFromQueue()
	}
	Logger.Printf("Executing %d jobs, %d in queue\n", len(FeedList), len(FeedQueue))
}
