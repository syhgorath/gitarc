// web/modules/project-modals.js - Proje yöneticisi modalı: ekleme, klasör seçimi, listeden kaldırma
import { state, el } from './state.js';
import { showToast } from './utils.js';
import { apiCall } from './api.js';

// Project Manager Modal Handlers
export function openAddProjectModal() {
  if (el.addProjectModal) el.addProjectModal.hidden = false;
  if (el.projectPathInput) {
    el.projectPathInput.value = '';
    el.projectPathInput.focus();
  }
  if (el.projectNameInput) el.projectNameInput.value = '';
}

export function closeAddProjectModal() {
  if (el.addProjectModal) el.addProjectModal.hidden = true;
}

export async function handleBrowseFolderClick() {
  const btn = document.getElementById('btn-browse-folder');
  if (btn) btn.disabled = true;

  try {
    const res = await apiCall('/api/system/choose-folder', 'POST', {});
    if (res && res.path) {
      if (el.projectPathInput) el.projectPathInput.value = res.path;
      if (el.projectNameInput && !el.projectNameInput.value) {
        el.projectNameInput.value = res.name || '';
      }
      showToast(`✓ Klasör seçildi: ${res.name}`, 'ok');
    }
  } catch (err) {
    const fallbackPicker = document.getElementById('fallback-folder-picker');
    if (fallbackPicker) {
      fallbackPicker.click();
    } else {
      showToast(`Klasör seçilemedi: ${err.message}`, 'warn');
    }
  } finally {
    if (btn) btn.disabled = false;
  }
}

export function handleFallbackFolderPicker(ev) {
  const files = ev.target.files;
  if (!files || files.length === 0) return;
  const first = files[0];
  const relativePath = first.webkitRelativePath || '';
  const folderName = relativePath.split('/')[0] || '';
  if (folderName && el.projectNameInput && !el.projectNameInput.value) {
    el.projectNameInput.value = folderName;
  }
  showToast(`✓ Klasör algılandı: ${folderName}`, 'info');
}

export async function handleAddProjectSubmit(ev, onApplySnapshot = null) {
  ev.preventDefault();
  const repoPath = el.projectPathInput?.value.trim() || '';
  const repoName = el.projectNameInput?.value.trim() || '';
  if (!repoPath) {
    showToast('Lütfen geçerli bir yerel depo yolu girin', 'warn');
    return;
  }
  const btn = document.getElementById('add-proj-submit');
  if (btn) btn.disabled = true;

  try {
    let snap;
    if (state.offlineMode) {
      snap = window.LocalFallbackEngine?.addProject(repoPath, repoName) || state.snapshot;
    } else {
      snap = await apiCall('/api/repo/add', 'POST', { path: repoPath, name: repoName });
    }
    closeAddProjectModal();
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast(`✓ Proje başarıyla eklendi: ${snap.repo.name}`, 'ok');
  } catch (err) {
    showToast(`Proje eklenemedi: ${err.message}`, 'danger');
  } finally {
    if (btn) btn.disabled = false;
  }
}

export async function handleRemoveCurrentProject(onApplySnapshot = null) {
  const currentRepo = state.snapshot?.repo;
  if (!currentRepo || String(currentRepo.id).startsWith('sim-')) {
    showToast('Simülasyon deposu listeden kaldırılamaz', 'warn');
    return;
  }
  const ok = window.confirm(`"${currentRepo.name}" projesini GitArc listesinden kaldırmak istiyor musunuz?\n\n(Not: Disk üzerindeki yerel dosyalarınız veya git deponuz ASLA silinmez, yalnızca GitArc proje listesinden çıkarılır.)`);
  if (!ok) return;

  try {
    let snap;
    if (state.offlineMode) {
      snap = window.LocalFallbackEngine?.removeProject(currentRepo.id) || state.snapshot;
    } else {
      snap = await apiCall('/api/repo/remove', 'POST', { repoId: currentRepo.id });
    }
    if (onApplySnapshot) onApplySnapshot(snap);
    showToast('Proje listeden kaldırıldı', 'info');
  } catch (err) {
    showToast(`Hata: ${err.message}`, 'danger');
  }
}
