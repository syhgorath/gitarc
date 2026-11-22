---
name: ci-fix
description: CI (GitHub Actions) bir PR dalında başarısız olduğunda kullan. Hatayı bulur, o dalın AYRI bir git worktree'sinde düzeltme commit'i atar, push yapmaz. "CI düştü", "hatalı CI", "kuyruk koşusu CI yüzünden durdu", "kırmızı PR" gibi durumlarda tetiklenir.
---

# CI düzeltme akışı (her proje için)

Amaç: düşen CI'ı **yalnızca o dalda** düzeltmek, kullanıcının çalışma ağacına ve diğer PR'lara dokunmadan. Kullanıcı commit'i inceleyip kendi aracından (GitArc vb.) pushlar.

## 1. Hatayı bul (ham log kuyruğuna güvenme)
- Önce projenin kendi aracını dene: GitArc varsa `gitarc_ci_failure_digest` / Hata Radarı.
- Yoksa: `gh run list --branch <dal> --limit 5`, sonra
  `gh run view <run-id> --log-failed | grep -nE -- "--- FAIL|panic|FAIL|_test.go:[0-9]+|Error:"`
- Uzun `go test -v` logunda `--- FAIL` ORTADA kalır, kuyruk yalnızca PASS gösterir. Kuyruğa bakıp "hata yok" deme.

## 2. Sınıflandır (düzeltmeden önce)
- **Flaky / zamanlama** (aynı test başka koşuda geçmiş, makine yavaşlığı, sıra bağımlılığı): önce yerelde `-count=10 -race` ile dene; üretemezsen nedeni koddaki gerçek bir yarıştan çıkar, "yeniden koştur" tek başına çözüm değildir ama sonuçta bunu söyle.
- **Mekanik** (format, vet, derleme, eksik CHANGELOG, `node --check`): doğrudan düzelt.
- **Mantık** (test gerçekten yanlış sonuç veriyor, tasarım kararı gerekiyor): düzeltmeye kalkma, bulguyu raporla ve kullanıcıya sor.
- Hata PR'ın kendi değişikliğinden mi geliyor, `main`'de de var mı? `main` zaten kırıksa PR dalına düzeltme koyma, bildir.

## 3. Düzeltme: dala GEÇME, worktree aç
- Kullanıcının çalışma ağacında bitmemiş iş olabilir. `git checkout`, `git switch`, `git stash` KULLANMA.
- `git fetch origin && git worktree add ../<proje>-<kisa-ad> <dal>` (dal başka worktree'de açıksa onu kullan, yoksa yeni aç).
- Çalışma bittikten sonra `git worktree remove` ile temizle (commit'ler dalda kalır).

## 4. Kapsam
- Yalnızca o dalın commit'i. `main`'e, başka dallara, `.github/` workflow dosyalarına, dal korumasına dokunma.
- En fazla 2 deneme. Düzelmezse dur, ne gördüğünü ve neyi denediğini raporla.

## 5. Kanıt ve doğrulama
- Düzeltmeyi geçici kapat: yeni/ilgili test DÜŞMELİ; aç: geçmeli. (Yeşil tek başına kanıt değil.)
- Projenin tam doğrulama komutunu çalıştır (AGENTS.md / CLAUDE.md / CI workflow'unda yazar; CI ile aynı bayraklarla, örn. `-race`). Çıktıda hata varsa özetle, yeşil demeden önce gör.
- Proje CHANGELOG tutuyorsa düzeltmeyi doğru başlığın altına ekle (PR metni oradan üretilebilir).

## 6. Commit, push yok
- Yerel commit at (proje kuralı farklıysa ona uy). **Push etme, PR açma/kapatma, merge etme.**
- Raporda şunları yaz: hangi dal, commit hash, hatanın kök nedeni, neyin doğrulandığı, neyin doğrulanmadığı, kullanıcının yapacağı (pushla, CI'ı izle).
- CI yeşile dönerse bile aralıklı hatalarda tek yeşil koşuyu "kesin düzeldi" diye sunma.

## 7. Ucuz modele devretme (isteğe bağlı)
Hata **mekanik** ise küçük bir alt ajana (worktree + yukarıdaki kurallarla) devredip kendi işine dönebilirsin; **mantık** hatasını devretme. Alt ajanın commit'ini sonuç olarak doğrula, sözüne güvenme.
