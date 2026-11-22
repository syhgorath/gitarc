package gitengine

import (
	"fmt"
	"strings"
	"testing"
)

// CI'da görülen durum: uzun `go test -v -race` logunda `--- FAIL` ORTADA kalır, sonrasında yüzlerce PASS satırı gelir.
// Eski davranış (son 16 KB'ı al, sonra ayıkla) bu hatayı hiç göremiyor ve panel "0 bulgu" diyordu.
func longRaceLog() string {
	var b strings.Builder
	p := "Go Vet & Test\tRun tests\t2026-11-22T08:42:48.9Z "
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "%s=== RUN   TestEarly%d\n%s--- PASS: TestEarly%d (0.01s)\n", p, i, p, i)
	}
	b.WriteString(p + "    queue_run_test.go:411: iki PR de birleşmeli: blocked\n")
	b.WriteString(p + "--- FAIL: TestQueueRunLiveMerges (1.10s)\n")
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&b, "%s=== RUN   TestLate%d\n%s--- PASS: TestLate%d (0.26s)\n", p, i, p, i)
	}
	b.WriteString(p + "FAIL\tgitarc/internal/gitengine\t52.836s\n")
	return b.String()
}

func TestCondenseKeepsTheFailureThatATailCutWouldLose(t *testing.T) {
	log := longRaceLog()
	if len(log) < 40000 {
		t.Fatalf("test günlüğü yeterince uzun değil: %d", len(log))
	}
	// eski yol: son 16 KB → hata görünmez
	if got := ExtractCILog(log[len(log)-16000:]); len(got) != 0 {
		t.Fatalf("önkoşul: kesik kuyrukta hata görünmemeli (görünüyor: %+v)", got)
	}
	cond := CondenseCILog(log, 16000)
	if len(cond) > 16000 {
		t.Fatalf("sınır aşıldı: %d", len(cond))
	}
	ex := ExtractCILog(cond)
	if len(ex) != 1 || ex[0].Symbol != "TestQueueRunLiveMerges" || ex[0].FileLine != "queue_run_test.go:411" || ex[0].Class != "mantik" {
		t.Fatalf("sıkıştırılmış logdan hata çıkmalı: %+v", ex)
	}
	if !strings.Contains(cond, "satır atlandı") {
		t.Fatal("atlanan aralıklar işaretlenmeli")
	}
	// tam log da aynı sonucu verir
	if full := ExtractCILog(log); len(full) != 1 || full[0].Symbol != ex[0].Symbol {
		t.Fatalf("tam log: %+v", full)
	}
}

func TestCondenseShortLogIsUntouchedAndNoFailureFallsBackToTail(t *testing.T) {
	if got := CondenseCILog("kısa\nlog", 100); got != "kısa\nlog" {
		t.Fatalf("kısa log değişmemeli: %q", got)
	}
	long := strings.Repeat("sadece gürültü\n", 2000)
	got := CondenseCILog(long, 500)
	if len(got) != 500 || !strings.HasSuffix(long, got) {
		t.Fatal("hata satırı yoksa son N bayt alınmalı")
	}
}

func TestExtractClassifiesMechanicalVersusLogic(t *testing.T) {
	ex := ExtractCILog("main.go:12:3: undefined: foo\n--- FAIL: TestX (0.1s)\n")
	got := map[string]string{}
	for _, e := range ex {
		got[e.Kind] = e.Class
	}
	if got["compile_error"] != "mekanik" || got["test_fail"] != "mantik" {
		t.Fatalf("sınıflar: %+v", got)
	}
}
