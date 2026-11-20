#!/usr/bin/env bash
# GitArc deneme deposuna istediğin kadar deneme dalı/PR üretir.
#   Yerelde aç : yalnızca yerel dal + commit (push YOK; GitArc'ta "Yeni Yerel Dal" görünür)
#   Uzakta aç  : dal push edilir + GitHub'da PR açılır (merge ETMEZ)
# Kullanım:  ./scripts/deneme-pr.sh                 (sorar)
#            ./scripts/deneme-pr.sh -n 5 -m remote   (m: local | remote)
#            REPO=/baska/yol ./scripts/deneme-pr.sh
set -euo pipefail

REPO="${REPO:-$HOME/Developer/gitarc-kuyruk-deneme}"
N=""; MODE=""
while getopts "n:m:h" o; do
  case $o in
    n) N="$OPTARG" ;;
    m) MODE="$OPTARG" ;;
    *) sed -n '2,9p' "$0"; exit 0 ;;
  esac
done

cd "$REPO" || { echo "Depo bulunamadı: $REPO" >&2; exit 1; }
# Yanlışlıkla gerçek bir depoya PR yağdırmamak için yalnızca deneme deposunda çalışır.
if ! git remote get-url origin | grep -q "gitarc-kuyruk-deneme"; then
  echo "Bu betik yalnızca gitarc-kuyruk-deneme deposu için. origin: $(git remote get-url origin)" >&2; exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "Çalışma ağacı kirli; önce commit/stash edin." >&2; exit 1
fi

if [ -z "$N" ]; then read -r -p "Kaç PR/dal? " N; fi
case "$N" in ''|*[!0-9]*) echo "Geçerli bir sayı girin." >&2; exit 1 ;; esac
[ "$N" -ge 1 ] || { echo "En az 1." >&2; exit 1; }
if [ -z "$MODE" ]; then
  echo "1) Yerelde aç (push yok)"; echo "2) Uzakta aç (push + PR)"
  read -r -p "Seçim [1/2]: " c
  case "$c" in 1) MODE=local ;; 2) MODE=remote ;; *) echo "1 ya da 2." >&2; exit 1 ;; esac
fi
case "$MODE" in local|remote) ;; *) echo "-m local ya da remote olmalı." >&2; exit 1 ;; esac
if [ "$MODE" = remote ]; then command -v gh >/dev/null || { echo "gh gerekli." >&2; exit 1; }; fi

git fetch -q origin
START="$(git rev-parse --abbrev-ref HEAD)"
STAMP="$(date +%m%d-%H%M%S)"
made=0
for i in $(seq 1 "$N"); do
  ID="d${STAMP}-${i}"
  BR="feat/${ID}"
  # Her dal ayrı bir dosyayı değiştirir: dallar birbiriyle çakışmaz.
  git checkout -q -b "$BR" origin/main
  echo "$ID" > "deneme-${ID}.txt"
  git add "deneme-${ID}.txt"
  git commit -q -m "feat: deneme-${ID} ekle"
  if [ "$MODE" = remote ]; then
    git push -q -u origin "$BR" 2>/dev/null
    URL="$(gh pr create --base main --head "$BR" --title "feat: deneme-${ID} ekle (deneme)" --body "GitArc deneme PR'ı (otomatik üretildi).")"
    echo "✓ $BR → $URL"
  else
    echo "✓ $BR (yerel)"
  fi
  made=$((made+1))
done
git checkout -q "$START"
echo "Bitti: $made $([ "$MODE" = remote ] && echo 'PR açıldı' || echo 'yerel dal oluşturuldu')."
