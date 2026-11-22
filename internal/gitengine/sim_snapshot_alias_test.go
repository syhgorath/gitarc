package gitengine

import (
	"encoding/json"
	"sync"
	"testing"
)

// Anlık görüntü sanal durumla işaretçi paylaşmamalı: sonradan yapılan bir işlem (burada FixCI: CI durumunu yazar) önceden alınmış
// görüntüyü DEĞİŞTİRMEMELİ. Paylaşım, arka plandaki kuyruk koşusu ile HTTP yanıtı arasında veri yarışına yol açıyordu (CI'da `-race`).
func TestSimulationSnapshotDoesNotShareStateWithLaterMutations(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, b := range snap.Branches {
		if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
			id = b.ID
			break
		}
	}
	if id == "" {
		t.Skip("demo verisinde CI'ı kırmızı PR yok")
	}
	before, _ := json.Marshal(snap)
	if _, err := mgr.FixCI(id); err != nil {
		t.Fatalf("FixCI: %v", err)
	}
	after, _ := json.Marshal(snap)
	if string(before) != string(after) {
		t.Fatal("önceden alınan anlık görüntü sonraki bir işlemle DEĞİŞTİ (işaretçi paylaşımı)")
	}
	fresh, _ := mgr.Snapshot()
	for _, b := range fresh.Branches {
		if b.ID == id && (b.PR == nil || b.PR.CI == nil || b.PR.CI.State != "SUCCESS") {
			t.Fatalf("yeni görüntü güncel durumu göstermeli: %+v", b.PR)
		}
	}
}

// `-race` altında: görüntüyü JSON'a çevirirken eşzamanlı işlem yapılması yarış üretmemeli.
func TestSimulationSnapshotEncodingRacesWithNoMutation(t *testing.T) {
	mgr := NewManager("")
	snap, _ := mgr.Snapshot()
	var id string
	for _, b := range snap.Branches {
		if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
			id = b.ID
		}
	}
	if id == "" {
		t.Skip("CI'ı kırmızı PR yok")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = json.Marshal(snap)
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = mgr.FixCI(id)
	}()
	wg.Wait()
}
