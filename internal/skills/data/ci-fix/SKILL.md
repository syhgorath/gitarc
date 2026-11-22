---
name: ci-fix
description: CI (GitHub Actions) bir PR dalında başarısız olduğunda kullan. Hatanın kaynağını ayırt eder (PR'ın kendi değişikliği, geride kalmış dal, kırık main, aralıklı test), uygun olanı o dalın AYRI bir git worktree'sinde yeni bir commit'le düzeltir, push yapmaz. "CI düştü", "hatalı CI", "kuyruk koşusu CI yüzünden durdu", "kırmızı PR", "check failed" gibi durumlarda tetiklenir.
---

# CI düzeltme akışı (her proje için)

Amaç: düşen CI'ı **doğru yerde** düzeltmek: çoğunlukla düşen PR'ın kendi dalında, kullanıcının çalışma ağacına ve diğer PR'lara dokunmadan. Kullanıcı commit'i inceleyip kendi aracından (GitArc vb.) pushlar.

## 0. GitHub'ın CI'ı nasıl çalıştırdığını bil
- `pull_request` iş akışları, PR dalının ucunu değil **PR ile hedef dalın (main) birleşmiş hâlini** (`refs/pull/N/merge`) test eder. Bu yüzden hata PR'ın kendi kodundan değil, **main'in o sırada ilerlemiş olmasından** da gelebilir.
- Yeni bir commit push'u CI'ı o PR'da yeniden başlatır. Gerekli kontrollerin (required checks) hepsi yeşil olmadan PR birleşmez.
- İnceleme kuralı "yeni commit onayı düşürür" ise push sonrası yeniden onay gerekebilir; kullanıcıya söyle.

## 1. Hatayı bul (ham log kuyruğuna güvenme)
- Önce projenin kendi aracını dene: GitArc varsa `gitarc_ci_failure_digest` / Hata Radarı.
- Yoksa: `gh pr checks <pr>` (hangi kontrol düştü), sonra
  `gh run view <run-id> --log-failed | grep -nE -- "--- FAIL|panic|FAIL|_test.go:[0-9]+|Error:"`
- Uzun `go test -v` logunda `--- FAIL` ORTADA kalır, kuyruk yalnızca PASS gösterir. Kuyruğa bakıp "hata yok" deme.
- Hatayı okumadan neden uydurma. Kök nedeni log satırına ve koda dayandır.

## 2. Kaynağı sınıflandır ve doğru hamleyi seç
| Durum | Hamle |
|---|---|
| **PR'ın kendi değişikliği** (yeni kod/test bozuk) | O dalda yeni commitle düzelt (aşağıdaki 3-6). |
| **Dal geride kaldı** (main'deki değişiklikle uyumsuz; `git log HEAD..origin/main` boş değil, hata birleşme sonucunda çıkıyor) | Önce dalı main ile güncelle (GitHub "Update branch" / `git merge origin/main`, rebase+force-push DEĞİL), tekrar test et. Çakışma varsa kullanıcıya bırak. |
| **main zaten kırık** (`gh run list --branch main --limit 3` kırmızı, aynı test orada da düşüyor) | PR dalına düzeltme KOYMA. Bildir; düzeltme `main` için ayrı bir PR'dır. PR, main düzelince güncellenir. |
| **Aralıklı (flaky)** (aynı SHA'da bir koşu geçti, bir koşu düştü; zamanlama/sıra bağımlı) | Önce yerelde `-count=10 -race` ile üretmeye çalış. Üretirsen kodda/testte gerçek bir yarış vardır: düzelt (yalnızca yeniden koştur demek çözüm değildir). Üretemezsen `gh run rerun <run-id> --failed` öner ve bunun bir tahmin olduğunu söyle. |
| **Mekanik** (format, vet, derleme, eksik CHANGELOG, `node --check`) | Doğrudan düzelt. |
| **Mantık/tasarım** (test gerçekten yanlış sonuç veriyor, karar gerekiyor) | Düzeltmeye kalkma; bulguyu ve seçenekleri raporla, kullanıcıya sor. |
| **Fork'tan gelen PR** | Dala push izniniz olmayabilir ("maintainers can edit" kapalıysa). Commit atma; neyin değişmesi gerektiğini yorum/rapor olarak ver. |

## 3. Düzeltme: dala GEÇME, worktree aç
- Kullanıcının çalışma ağacında bitmemiş iş olabilir. `git checkout`, `git switch`, `git stash` KULLANMA.
- `git fetch origin && git worktree add ../<proje>-<kisa-ad> <dal>` (dal başka worktree'de açıksa onu kullan).
- Bittikten sonra `git worktree remove` ile temizle (commit'ler dalda kalır).

## 4. Kapsam
- Yalnızca o dalın commit'i. `main`'e, başka dallara, `.github/` workflow dosyalarına, dal korumasına dokunma.
- **Yeni commit at. `--amend`, rebase ya da force-push yapma:** geçmiş korunur, push normal kalır, PR yorumları/incelemeler kaybolmaz.
- En fazla 2 deneme. Düzelmezse dur, ne gördüğünü ve neyi denediğini raporla.

## 5. Kanıt ve doğrulama
- Düzeltmeyi geçici kapat: ilgili test DÜŞMELİ; aç: geçmeli. (Yeşil tek başına kanıt değil.)
- Projenin tam doğrulama komutunu CI ile aynı bayraklarla çalıştır (AGENTS.md / CLAUDE.md / workflow'da yazar; örn. `-race`). Yeşil demeden önce çıktıyı gör.
- Proje CHANGELOG tutuyorsa düzeltmeyi doğru başlığın altına ekle (PR metni oradan üretilebilir).

## 6. Commit, push yok
- Yerel commit at (proje kuralı farklıysa ona uy). **Push etme, PR açma/kapatma, merge etme, CI'ı kendiliğinden yeniden koşturma.**
- Raporda şunları yaz: hangi dal, commit hash, hatanın kök nedeni ve **hangi sınıf** olduğu, neyin doğrulandığı, neyin doğrulanmadığı, kullanıcının yapacağı (pushla, CI'ı izle).
- Aralıklı hatada tek yeşil koşuyu "kesin düzeldi" diye sunma.
- Birleştirme kuyruğu koşusu "blocked" ile durduysa: push + CI yeşil olunca koşuyu **yeniden başlatmak** gerekir (blocked kalıcıdır); bunu rapora yaz.

## 7. Ucuz modele devretme (isteğe bağlı)
Hata **mekanik** ise küçük bir alt ajana (worktree + yukarıdaki kurallarla) devredip kendi işine dönebilirsin; **mantık** ve **main kırık** durumlarını devretme. Alt ajanın commit'ini sonuç olarak doğrula, sözüne güvenme.
