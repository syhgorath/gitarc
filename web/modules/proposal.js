// web/modules/proposal.js - AI commit/PR önerisi kartının olayları (taslak kaydı, onay, reddet)
// Onay yükünün doğrulaması saf modeldedir (inspector-model.js `buildApprovePayload`).
import { state } from './state.js';
import { showToast } from './utils.js';
import { apiCall } from './api.js';
import { buildApprovePayload } from './inspector-model.js';

export function wireProposalActions(onApplySnapshot = null) {
  const prop = state.snapshot?.pendingProposal;
  if (!prop) return;

  const bIn = document.getElementById('prop-edit-branch');
  const mIn = document.getElementById('prop-edit-message');
  const tIn = document.getElementById('prop-edit-title');
  const dIn = document.getElementById('prop-edit-body');

  const saveDraft = () => {
    state.proposalDraftEdits = {
      id: prop.id,
      branch: bIn ? bIn.value.trim() : prop.branchName,
      message: mIn ? mIn.value : prop.message,
      title: tIn ? tIn.value.trim() : prop.prTitle,
      body: dIn ? dIn.value : prop.prBody,
    };
  };

  bIn?.addEventListener('input', saveDraft);
  mIn?.addEventListener('input', saveDraft);
  tIn?.addEventListener('input', saveDraft);
  dIn?.addEventListener('input', saveDraft);

  document.getElementById('act-proposal-approve')?.addEventListener('click', async () => {
    const fields = {
      branch: bIn ? bIn.value.trim() : prop.branchName,
      message: mIn ? mIn.value.trim() : prop.message,
      title: tIn ? tIn.value.trim() : prop.prTitle,
      body: dIn ? dIn.value : prop.prBody,
    };
    const built = buildApprovePayload(prop, fields);
    if (built.error) {
      showToast(built.error, 'danger');
      return;
    }
    const { payload } = built;
    const { branchName } = payload;

    if (!state.offlineMode) {
      try {
        const snap = await apiCall('/api/proposal/approve', 'POST', payload);
        delete state.proposalDraftEdits;
        showToast(`✓ AI önerisi onaylandı: ${branchName} dalı oluşturuldu ve PR açıldı!`, 'ok');
        if (onApplySnapshot) onApplySnapshot(snap);
      } catch (err) {
        showToast(`Hata: ${err.message}`, 'danger');
      }
    } else if (window.LocalFallbackEngine) {
      try {
        const snap = window.LocalFallbackEngine.approveProposal(payload);
        delete state.proposalDraftEdits;
        showToast(`✓ AI önerisi onaylandı (Offline Mod): ${branchName}`, 'ok');
        if (onApplySnapshot) onApplySnapshot(snap);
      } catch (err) {
        showToast(`Hata: ${err.message}`, 'danger');
      }
    }
  });

  document.getElementById('act-proposal-dismiss')?.addEventListener('click', async () => {
    delete state.proposalDraftEdits;
    showToast('AI commit/PR önerisi reddedildi (dosyalarınız korundu)', 'info');
    if (!state.offlineMode) {
      try {
        const dismissedSnap = await apiCall('/api/proposal/dismiss', 'POST', {});
        if (onApplySnapshot) onApplySnapshot(dismissedSnap);
      } catch (_) {}
    } else if (window.LocalFallbackEngine) {
      window.LocalFallbackEngine.dismissProposal();
      if (onApplySnapshot) onApplySnapshot(state.snapshot);
    }
  });
}
