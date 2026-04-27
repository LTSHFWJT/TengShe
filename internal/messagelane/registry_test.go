package messagelane

import "testing"

func TestRegistryResolveMatchesRequestID(t *testing.T) {
	registry := NewRegistry[string]()

	firstID, firstCh, firstCancel := registry.Open()
	defer firstCancel()
	secondID, secondCh, secondCancel := registry.Open()
	defer secondCancel()

	if firstID == NoID || secondID == NoID || firstID == secondID {
		t.Fatalf("request ids are not unique: first=%d second=%d", firstID, secondID)
	}

	if !registry.Resolve(secondID, "second") {
		t.Fatal("failed to resolve second request")
	}
	if got := <-secondCh; got != "second" {
		t.Fatalf("second response = %q, want second", got)
	}

	if !registry.Resolve(firstID, "first") {
		t.Fatal("failed to resolve first request")
	}
	if got := <-firstCh; got != "first" {
		t.Fatalf("first response = %q, want first", got)
	}

	if registry.Len() != 0 {
		t.Fatalf("pending len = %d, want 0", registry.Len())
	}
}

func TestRegistryCancelClosesLane(t *testing.T) {
	registry := NewRegistry[int]()

	id, ch, cancel := registry.Open()
	cancel()

	if _, ok := <-ch; ok {
		t.Fatal("cancelled lane is still open")
	}
	if registry.Resolve(id, 1) {
		t.Fatal("cancelled request should not resolve")
	}
}
