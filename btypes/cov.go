package btypes

// SubscribeCOVData holds parameters for a SubscribeCOV confirmed service request.
//
// When Cancellation is true (or IssueConfirmedNotifications/Lifetime are omitted
// on the wire), the request cancels an existing subscription.
//
// 中文说明：SubscribeCOVData 表示 SubscribeCOV 确认服务请求参数。
// Cancellation 为 true 时表示取消订阅。
type SubscribeCOVData struct {
	ProcessID                   uint32
	ObjectID                    ObjectID
	IssueConfirmedNotifications bool
	Lifetime                    uint32 // seconds; 0 with Cancellation means cancel
	Cancellation                bool
}

// COVNotification is a Confirmed or Unconfirmed COV Notification payload.
//
// 中文说明：COVNotification 表示确认/非确认 COV 通知负载。
type COVNotification struct {
	ProcessID            uint32
	InitiatingDeviceID   ObjectID
	MonitoredObjectID    ObjectID
	TimeRemaining        uint32
	ListOfValues         []Property
	Confirmed            bool // true = ConfirmedCOVNotification
}
