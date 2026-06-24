package kit

import (
	"testing"

	"gitee.com/linakesi/lzc-sdk/lang/go/common"
)

func TestNewNotifyRequestTrimsAndValidatesContent(t *testing.T) {
	req, err := newNotifyRequest("  title  ", "  body  ", "  lzc://app/cloud.lazycat.app.demo  ")
	if err != nil {
		t.Fatal(err)
	}
	if req.Title != "title" || req.Body != "body" || req.GetDeeplinkUrl() != "lzc://app/cloud.lazycat.app.demo" {
		t.Fatalf("unexpected request: %#v", req)
	}

	if _, err := newNotifyRequest(" ", "body", ""); err == nil {
		t.Fatal("expected empty title error")
	}
	if _, err := newNotifyRequest("title", " ", ""); err == nil {
		t.Fatal("expected empty body error")
	}
}

func TestResolveNotificationTargetsDefaultsToAllOnlineDevices(t *testing.T) {
	targets := []notificationTarget{
		{UID: "u1", Device: &common.EndDevice{UniqueDeivceId: "online-1", IsOnline: true}},
		{UID: "u1", Device: &common.EndDevice{UniqueDeivceId: "offline-1", IsOnline: false}},
		{UID: "u2", Device: &common.EndDevice{UniqueDeivceId: "online-2", IsOnline: true}},
	}

	got, err := resolveNotificationTargets(targets, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("target count = %d", len(got))
	}
	if got[0].Device.GetUniqueDeivceId() != "online-1" || got[1].Device.GetUniqueDeivceId() != "online-2" {
		t.Fatalf("unexpected targets: %#v", got)
	}
}

func TestResolveNotificationTargetsUsesExplicitDeviceIDs(t *testing.T) {
	targets := []notificationTarget{
		{UID: "u1", Device: &common.EndDevice{UniqueDeivceId: "online-1", IsOnline: true}},
		{UID: "u1", Device: &common.EndDevice{UniqueDeivceId: "offline-1", IsOnline: false}},
		{UID: "u2", Device: &common.EndDevice{UniqueDeivceId: "online-2", IsOnline: true}},
	}

	got, err := resolveNotificationTargets(targets, []string{"online-2", "online-2", " online-1 "})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("target count = %d", len(got))
	}
	if got[0].Device.GetUniqueDeivceId() != "online-1" || got[1].Device.GetUniqueDeivceId() != "online-2" {
		t.Fatalf("unexpected targets: %#v", got)
	}

	if _, err := resolveNotificationTargets(targets, []string{"offline-1"}); err == nil {
		t.Fatal("expected offline device error")
	}
	if _, err := resolveNotificationTargets(targets, []string{"missing"}); err == nil {
		t.Fatal("expected missing device error")
	}
}
