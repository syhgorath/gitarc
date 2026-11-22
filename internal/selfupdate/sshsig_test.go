package selfupdate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testSigners(t *testing.T) []Signer {
	t.Helper()
	s := ParseAllowedSigners(string(fixture(t, "signers")))
	if len(s) != 1 {
		t.Fatalf("test imzacısı okunamadı: %d", len(s))
	}
	return s
}

func TestVerifyAcceptsAValidSignatureAndRejectsEverythingElse(t *testing.T) {
	signers := testSigners(t)
	data := fixture(t, "fake-binary")
	if err := Verify(signers, Principal, Namespace, fixture(t, "fake-binary.sig"), bytes.NewReader(data)); err != nil {
		t.Fatalf("geçerli imza kabul edilmeli: %v", err)
	}
	// içerik değişti
	tampered := append([]byte(nil), data...)
	tampered[0] ^= 1
	if err := Verify(signers, Principal, Namespace, fixture(t, "fake-binary.sig"), bytes.NewReader(tampered)); err != ErrBadSig {
		t.Fatalf("değiştirilmiş içerik reddedilmeli: %v", err)
	}
	// başka ad alanıyla imzalanmış (aynı anahtar): GitArc sürümü olarak KABUL EDİLMEZ
	if err := Verify(signers, Principal, Namespace, fixture(t, "fake-binary.wrongns.sig"), bytes.NewReader(data)); err != ErrNamespace {
		t.Fatalf("yanlış ad alanı reddedilmeli: %v", err)
	}
	// izinli listede olmayan anahtarla imzalanmış
	if err := Verify(signers, Principal, Namespace, fixture(t, "fake-binary.otherkey.sig"), bytes.NewReader(data)); err != ErrNoSigner {
		t.Fatalf("tanınmayan imzacı reddedilmeli: %v", err)
	}
	// imzacı yok → hiçbir şey kabul edilmez (fail-closed)
	if err := Verify(nil, Principal, Namespace, fixture(t, "fake-binary.sig"), bytes.NewReader(data)); err != ErrNoSigner {
		t.Fatalf("imzacısız doğrulama reddedilmeli: %v", err)
	}
	// başka ilke
	if err := Verify(signers, "baska-ilke", Namespace, fixture(t, "fake-binary.sig"), bytes.NewReader(data)); err != ErrNoSigner {
		t.Fatalf("başka ilke reddedilmeli: %v", err)
	}
	// bozuk zarf
	for _, bad := range []string{"", "merhaba", "-----BEGIN SSH SIGNATURE-----\n!!!\n-----END SSH SIGNATURE-----\n"} {
		if err := Verify(signers, Principal, Namespace, []byte(bad), bytes.NewReader(data)); err == nil {
			t.Fatalf("bozuk imza kabul edildi: %q", bad)
		}
	}
	// kesilmiş imza
	sig := fixture(t, "fake-binary.sig")
	if err := Verify(signers, Principal, Namespace, sig[:len(sig)/2], bytes.NewReader(data)); err == nil {
		t.Fatal("kesilmiş imza kabul edildi")
	}
}

func TestParseAllowedSignersIgnoresBrokenAndUnsupportedLines(t *testing.T) {
	good := strings.TrimSpace(string(fixture(t, "signers")))
	key := strings.Fields(good)[3]
	text := strings.Join([]string{
		"# yorum", "",
		good,
		"gitarc-release ssh-rsa AAAA", // desteklenmeyen anahtar türü
		"gitarc-release valid-before=\"2030\" ssh-ed25519 " + key, // desteklenmeyen seçenek
		"*.example ssh-ed25519 " + key,                            // desen içeren ilke
		"sadece-bir-alan",
		`gitarc-release namespaces="" ssh-ed25519 ` + key, // boş ad alanı
		"gitarc-release ssh-ed25519 !!!bozuk!!!",
	}, "\n")
	got := ParseAllowedSigners(text)
	if len(got) != 1 || got[0].Principal != "gitarc-release" || len(got[0].Namespaces) != 1 {
		t.Fatalf("yalnızca geçerli tek satır okunmalı: %+v", got)
	}
}

// Gömülü imzacı dosyası anahtar içermiyorsa güncelleme KAPALIDIR; ve gömülü dosya asla test anahtarını içermez.
func TestEmbeddedSignersFileNeverContainsTheTestKey(t *testing.T) {
	test := strings.Fields(string(fixture(t, "signers")))[3]
	if strings.Contains(embeddedSigners, test) {
		t.Fatal("test anahtarı gömülü imzacı dosyasına girmemeli")
	}
}

// ssh-keygen referanstır: yeni anahtar + gerçek imza → Go kabul etmeli; içerik değişince ikisi de reddetmeli.
func TestVerifyAgreesWithSSHKeygen(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen yok")
	}
	dir := t.TempDir()
	priv := filepath.Join(dir, "k")
	if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "", "-f", priv).CombinedOutput(); err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
	pub, _ := os.ReadFile(priv + ".pub")
	f := strings.Fields(string(pub))
	signersText := `gitarc-release namespaces="gitarc-release" ` + f[0] + " " + f[1] + "\n"
	data := []byte("gerçek imza denemesi\n")
	df := filepath.Join(dir, "d")
	_ = os.WriteFile(df, data, 0o600)
	if out, err := exec.Command("ssh-keygen", "-Y", "sign", "-f", priv, "-n", Namespace, df).CombinedOutput(); err != nil {
		t.Fatalf("sign: %v %s", err, out)
	}
	sig, _ := os.ReadFile(df + ".sig")
	if err := Verify(ParseAllowedSigners(signersText), Principal, Namespace, sig, bytes.NewReader(data)); err != nil {
		t.Fatalf("ssh-keygen'in imzaladığı geçerli dosya Go'da kabul edilmeli: %v", err)
	}
	if err := Verify(ParseAllowedSigners(signersText), Principal, Namespace, sig, bytes.NewReader(append(data, 'x'))); err == nil {
		t.Fatal("değişmiş içerik kabul edildi")
	}
}
