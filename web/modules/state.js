// web/modules/state.js - Reactive state store and cached DOM element registry

export const state = {
  snapshot: null,
  selectedBranchId: null,
  selectedNodeSha: null,
  filter: 'all', // 'all' | 'mine' | 'issues' | 'ci_fail' | 'lower' | 'gone'
  pathLens: '',
  showMergedArches: true,
  mergeStrategy: 'merge',
  offlineMode: false,
  lastSig: '',
  zoom: 1,
  panX: 0,
  panY: 0,
  baseOpen: false,      // sol raydaki omurga dalı seçici açık mı
  longLivedOpen: false, // sol raydaki "Uzun Ömürlü" çipi açık mı
  historyOpen: false,    // sol raydaki "Geçmiş" çipi açık mı
  anchorEnd: true,      // ilk çizimde en güncel (sağ) bölgeye yaslan
  anchoredRepoId: null,
  unitsPerPx: 0,
  panning: null,
  drag: null,
  localRunOutput: {},
  pendingAutoMerge: {},
  autoMergingInFlight: {},
  draftPrEdits: {},
  studio: null,
  clientLogs: [],
  radarDismissed: {}, // Hata Radarı'nda kapatılan işlem hatası kartları: grup anahtarı → kapatıldığı andaki son günlük kimliği
};

// Projeye özel arayüz durumu: proje değişince eski projenin seçimi, taslakları ve KUYRUĞA ALINMIŞ otomatik merge'leri yeni
// projeye taşınmamalı (dal adı eşleşirse başka projedeki bir dal birleştirilirdi).
export function resetProjectScopedState(st = state) {
  st.selectedBranchId = null;
  st.selectedNodeSha = null;
  st.pendingAutoMerge = {};
  st.radarDismissed = {};
  st.autoMergingInFlight = {};
  st.draftPrEdits = {};
  st.proposalDraftEdits = {};
  st.localRunOutput = {};
  st.studio = null;
  st.baseOpen = false;
  st.longLivedOpen = false;
  st.historyOpen = false;
}

// Cached DOM element lookup helper
export const el = {};

export function initElements() {
  const ids = [
    'rail-svg', 'svg-viewport', 'repo-select', 'mode-badge', 'kpi-pills',
    'radar-list', 'bottom-dock', 'dock-tab-radar', 'dock-tab-logs',
    'dock-tab-actions', 'dock-radar-count', 'dock-log-count', 'dock-action-count',
    'dock-toggle-btn', 'dock-clear-btn', 'dock-pane-radar', 'dock-pane-logs',
    'dock-pane-actions', 'dock-logs-stream', 'dock-actions-stream', 'inspector-detail',
    'prune-count', 'drag-hint', 'zoom-reset',
    'toggle-merged-arches', 'create-branch-modal', 'new-branch-form', 'new-branch-name',
    'new-branch-base', 'toast-stack', 'conflict-studio-modal', 'studio-branch-badge',
    'studio-file-tabs', 'studio-guide-box', 'studio-guide-text', 'studio-strategy-bar',
    'studio-line-badge', 'studio-mode-badge', 'studio-left-conflict', 'studio-right-preview',
    'studio-error-banner', 'studio-error-text', 'studio-run-vet', 'studio-apply',
    'mcp-connect-modal', 'mcp-config-snippet', 'mcp-gated',
    'btn-queue', 'queue-modal', 'queue-candidates', 'queue-result',
    'logs-modal', 'logs-list', 'btn-protect-branch', 'btn-main-ci', 'btn-no-project-add', 'loading-hint', 'btn-update-check',
    'lens-path-input', 'perf-cache-badge', 'security-update-modal', 'sec-prot-badge',
    'sec-prot-summary', 'prot-profiles', 'prot-checks', 'prot-now', 'prot-changes', 'prot-notes', 'btn-enable-protection', 'sec-team-chip', 'sec-update-status',
    'sec-update-cmd', 'btn-add-project', 'btn-remove-project', 'add-project-modal',
    'add-project-form', 'project-path-input', 'project-name-input', 'add-proj-close',
    'add-proj-cancel', 'btn-start-tour', 'tour-backdrop', 'tour-card',
    'tour-step-badge', 'tour-step-title', 'tour-step-body', 'tour-prev-btn',
    'tour-next-btn', 'tour-dots', 'tour-close-btn',
    'btn-upper-ray', 'btn-lower-ray', 'upper-ray-pill-count', 'lower-ray-pill-count',
    'btn-canvas-sync', 'canvas-sync-dot', 'canvas-sync-text', 'dock-copy-btn',
    'canvas-inspector', 'canvas-inspector-close', 'canvas-inspector-title',
    'studio-branch-chip', 'studio-branch-detail', 'studio-mid-status',
    'studio-diff-meta', 'studio-diff-fullscreen', 'studio-diff-content',
    'diff-viewer-modal', 'diff-modal-close', 'diff-modal-foot-close', 'diff-stats-badge', 'diff-files-count', 'diff-files-list',
    'diff-target-label', 'diff-viewer-container', 'btn-copy-diff',
    'studio-col-left', 'studio-col-mid', 'studio-col-right'
  ];

  ids.forEach((id) => {
    // Convert kebab-case to camelCase for el map (e.g. 'rail-svg' -> 'svg', 'inspector-detail' -> 'detail')
    const dom = document.getElementById(id);
    el[id] = dom;
  });

  // Semantic alias mappings to match existing codebase conventions
  el.svg = el['rail-svg'];
  el.viewport = el['svg-viewport'];
  el.repoSelect = el['repo-select'];
  el.studioBranchChip = el['studio-branch-chip'];
  el.studioBranchDetail = el['studio-branch-detail'];
  el.studioMidStatus = el['studio-mid-status'];
  el.studioDiffMeta = el['studio-diff-meta'];
  el.studioDiffFullscreen = el['studio-diff-fullscreen'];
  el.studioDiffContent = el['studio-diff-content'];
  el.modeBadge = el['mode-badge'];
  el.kpiPills = el['kpi-pills'];
  el.radarList = el['radar-list'];
  el.dock = el['bottom-dock'];
  el.dockTabRadar = el['dock-tab-radar'];
  el.dockTabLogs = el['dock-tab-logs'];
  el.dockTabActions = el['dock-tab-actions'];
  el.dockRadarCount = el['dock-radar-count'];
  el.dockLogCount = el['dock-log-count'];
  el.dockActionCount = el['dock-action-count'];
  el.dockToggleBtn = el['dock-toggle-btn'];
  el.dockClearBtn = el['dock-clear-btn'];
  el.dockCopyBtn = el['dock-copy-btn'];
  el.dockPaneRadar = el['dock-pane-radar'];
  el.dockPaneLogs = el['dock-pane-logs'];
  el.dockPaneActions = el['dock-pane-actions'];
  el.dockLogsStream = el['dock-logs-stream'];
  el.dockActionsStream = el['dock-actions-stream'];
  el.detail = el['inspector-detail'];
  el.pruneCount = el['prune-count'];
  el.dragHint = el['drag-hint'];
  // Rozetin gerçek kimliği btn-canvas-sync'tir ('spine-sync-badge' yalnızca CSS sınıfıdır). Eski eşleme
  // hiçbir öğeyi bulamadığı için rozet güncellenmiyor, hep HTML'deki "(Synced)" metnini gösteriyordu.
  el.spineSyncBadge = el['btn-canvas-sync'];
  el.btnCanvasSync = el['btn-canvas-sync'];
  el.canvasSyncDot = el['canvas-sync-dot'];
  el.canvasSyncText = el['canvas-sync-text'];
  el.engineStatus = document.getElementById('engine-status');
  el.zoomReset = el['zoom-reset'];
  el.toggleMerged = el['toggle-merged-arches'];
  el.modal = el['create-branch-modal'];
  el.newBranchForm = el['new-branch-form'];
  el.newBranchName = el['new-branch-name'];
  el.newBranchBase = el['new-branch-base'];
  el.toastStack = el['toast-stack'];
  el.conflictStudioModal = el['conflict-studio-modal'];
  el.studioBranchBadge = el['studio-branch-badge'];
  el.studioFileTabs = el['studio-file-tabs'];
  el.studioGuideBox = el['studio-guide-box'];
  el.studioGuideText = el['studio-guide-text'];
  el.studioStrategyBar = el['studio-strategy-bar'];
  el.studioLineBadge = el['studio-line-badge'];
  el.studioModeBadge = el['studio-mode-badge'];
  el.studioLeftConflict = el['studio-left-conflict'];
  el.studioRightPreview = el['studio-right-preview'];
  el.studioErrorBanner = el['studio-error-banner'];
  el.studioErrorText = el['studio-error-text'];
  el.studioRunVet = el['studio-run-vet'];
  el.studioApplyBtn = el['studio-apply'];
  el.mcpModal = el['mcp-connect-modal'];
  el.mcpGated = el['mcp-gated'];
  el.mcpConfigSnippet = el['mcp-config-snippet'];
  el.btnQueue = el['btn-queue'];
  el.queueModal = el['queue-modal'];
  el.queueCandidates = el['queue-candidates'];
  el.queueResult = el['queue-result'];
  el.diffModal = el['diff-viewer-modal'];
  el.diffModalClose = el['diff-modal-close'];
  el.diffModalFootClose = el['diff-modal-foot-close'];
  el.diffStatsBadge = el['diff-stats-badge'];
  el.diffFilesCount = el['diff-files-count'];
  el.diffFilesList = el['diff-files-list'];
  el.diffTargetLabel = el['diff-target-label'];
  el.diffViewerContainer = el['diff-viewer-container'];
  el.btnCopyDiff = el['btn-copy-diff'];
  el.logsModal = el['logs-modal'];
  el.logsList = el['logs-list'];
  el.btnProtectBranch = el['btn-protect-branch'];
  el.btnMainCI = el['btn-main-ci'];
  el.btnNoProjectAdd = el['btn-no-project-add'];
  el.loadingHint = el['loading-hint'];
  el.btnUpdateCheck = el['btn-update-check'];
  el.lensPathInput = el['lens-path-input'];
  el.perfCacheBadge = el['perf-cache-badge'];
  el.securityUpdateModal = el['security-update-modal'];
  el.secProtBadge = el['sec-prot-badge'];
  el.secProtSummary = el['sec-prot-summary'];
  el.protProfiles = el['prot-profiles'];
  el.protChecks = el['prot-checks'];
  el.protNow = el['prot-now'];
  el.protChanges = el['prot-changes'];
  el.protNotes = el['prot-notes'];
  el.btnEnableProtection = el['btn-enable-protection'];
  el.secTeamChip = el['sec-team-chip'];
  el.secUpdateStatus = el['sec-update-status'];
  el.secUpdateCmd = el['sec-update-cmd'];
  el.btnAddProject = el['btn-add-project'];
  el.btnRemoveProject = el['btn-remove-project'];
  el.addProjectModal = el['add-project-modal'];
  el.addProjectForm = el['add-project-form'];
  el.projectPathInput = el['project-path-input'];
  el.projectNameInput = el['project-name-input'];
  el.addProjClose = el['add-proj-close'];
  el.addProjCancel = el['add-proj-cancel'];
  el.btnStartTour = el['btn-start-tour'];
  el.tourBackdrop = el['tour-backdrop'];
  el.tourCard = el['tour-card'];
  el.tourStepBadge = el['tour-step-badge'];
  el.tourStepTitle = el['tour-step-title'];
  el.tourStepBody = el['tour-step-body'];
  el.tourPrevBtn = el['tour-prev-btn'];
  el.tourNextBtn = el['tour-next-btn'];
  el.tourDots = el['tour-dots'];
  el.tourCloseBtn = el['tour-close-btn'];

  el.btnUpperRay = el['btn-upper-ray'];
  el.btnLowerRay = el['btn-lower-ray'];
  el.upperRayCount = el['upper-ray-pill-count'];
  el.lowerRayCount = el['lower-ray-pill-count'];

  el.canvasInspector = el['canvas-inspector'];
  el.canvasInspectorClose = el['canvas-inspector-close'];
  el.canvasInspectorTitle = el['canvas-inspector-title'];

  return el;
}
