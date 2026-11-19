// web/modules/tour.js - Spotlight etkileşimli kokpit turu
import { el } from './state.js';

// Spotlight Interactive Tour Handlers
export const TOUR_STEPS = [
  {
    targetId: null,
    targetSelector: '.repo-picker',
    posClass: 'tour-pos-top-left',
    badge: 'Adım 1 / 5',
    title: '📂 Depo Seçimi & ➕ Proje Ekle',
    body: '<p>Buradan çalıştığınız canlı Git depoları veya simülasyon demoları arasında anında geçiş yapabilirsiniz.</p><p><strong>➕ Proje Ekle</strong> butonuna basarak bilgisayarınızdaki herhangi bir yerel Git projesini tek tıkla GitArc kokpitine dahil edebilirsiniz.</p>',
  },
  {
    targetId: null,
    targetSelector: null,
    posClass: 'tour-pos-center',
    badge: 'Adım 2 / 5',
    title: '🚀 Parabolik Ray Tuvali',
    body: '<p>Git yaşam döngünüz burada <strong>parabolik raylar</strong> olarak görselleştirilir:</p><p>• <strong>Merkez Ray:</strong> <code>main</code> omurgası, commitler ve sürüm tag\'leri.<br>• <strong>Üst Ray:</strong> GitHub Remote ve Pull Request durumları.<br>• <strong>Alt Ray:</strong> Henüz pushlanmamış yerel dallarınız ve kaydedilmemiş çalışma dizininiz.</p>',
  },
  {
    targetId: 'bottom-dock',
    targetSelector: '#bottom-dock',
    posClass: 'tour-pos-bottom-dock',
    badge: 'Adım 3 / 5',
    title: '🚨 PocketUI Alt Çekmece (Radar & Loglar)',
    body: '<p><strong>Hata & Uyarı Radarı</strong> ekranın altında tam genişlikte çalışır. CI kırılmaları, merge çakışmaları ve silinmiş uzak dallar etkilenen dosya etiketleriyle burada listelenir.</p><p>Ayrıca <strong>📜 Loglar</strong> ve <strong>⚡ Son İşlemler</strong> sekmeleriyle sistemin attığı tüm adımları canlı takip edebilirsiniz.</p>',
  },
  {
    targetId: 'inspector-detail',
    targetSelector: '.inspector-pane',
    posClass: 'tour-pos-inspector',
    badge: 'Adım 4 / 5',
    title: '🛠️ Dal, Çakışma & Commit Stüdyosu',
    body: '<p>Grafikten bir dal veya commit seçtiğinizde sağ panelde stüdyo açılır:</p><p>• <strong>Tek Tıkla Dala Geçiş (Checkout)</strong> ve PR bilgisi düzenleme.<br>• <strong>Çakışma Stüdyosu:</strong> İzole <code>git worktree</code> üzerinde kayıpsız çakışma çözümü.<br>• <strong>Konsolsuz Commit:</strong> Terminal açmadan yerel değişikliklerinizi commitleyip PR açma.</p>',
  },
  {
    targetId: null,
    targetSelector: '.topbar-right',
    posClass: 'tour-pos-top-right',
    badge: 'Adım 5 / 5',
    title: '🛡️ Güvenlik Kalkanı & MCP Entegrasyonu',
    body: '<p>GitArc, hassas verileri (secret/private key/env) depoya eklenmeden önce engeller.</p><p><strong>🔌 MCP</strong> butonu ile Claude Desktop, Cursor veya Antigravity gibi AI asistanlarınıza GitArc araçlarını bağlayabilirsiniz.</p>',
  },
];

let activeTourStep = 0;

export function renderTourStep(stepIdx) {
  if (stepIdx < 0 || stepIdx >= TOUR_STEPS.length) return;
  activeTourStep = stepIdx;
  const step = TOUR_STEPS[stepIdx];

  document.querySelectorAll('.tour-highlight').forEach((node) => node.classList.remove('tour-highlight'));

  if (step.targetSelector) {
    const targetEl = document.querySelector(step.targetSelector);
    if (targetEl) targetEl.classList.add('tour-highlight');
  }

  if (el.tourCard) {
    el.tourCard.className = `tour-card ${step.posClass || ''}`;
  }
  if (el.tourStepBadge) el.tourStepBadge.textContent = step.badge;
  if (el.tourStepTitle) el.tourStepTitle.textContent = step.title;
  if (el.tourStepBody) el.tourStepBody.innerHTML = step.body;

  if (el.tourPrevBtn) el.tourPrevBtn.disabled = stepIdx === 0;
  if (el.tourNextBtn) el.tourNextBtn.textContent = stepIdx === TOUR_STEPS.length - 1 ? 'Tamamla ✓' : 'Sonraki ▶';

  if (el.tourDots) {
    el.tourDots.innerHTML = '';
    TOUR_STEPS.forEach((_, idx) => {
      const dot = document.createElement('span');
      dot.className = `tour-dot ${idx === stepIdx ? 'active' : ''}`;
      el.tourDots.appendChild(dot);
    });
  }
}

export function startCockpitTour() {
  if (el.tourBackdrop) el.tourBackdrop.hidden = false;
  if (el.tourCard) el.tourCard.hidden = false;
  renderTourStep(0);
}

export function closeTour() {
  if (el.tourBackdrop) el.tourBackdrop.hidden = true;
  if (el.tourCard) el.tourCard.hidden = true;
  document.querySelectorAll('.tour-highlight').forEach((node) => node.classList.remove('tour-highlight'));
}

export function nextTourStep() {
  if (activeTourStep < TOUR_STEPS.length - 1) {
    renderTourStep(activeTourStep + 1);
  } else {
    closeTour();
  }
}

export function prevTourStep() {
  if (activeTourStep > 0) {
    renderTourStep(activeTourStep - 1);
  }
}
