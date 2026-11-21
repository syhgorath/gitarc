// web/modules/constants.js - GitArc layout, geometry and system constants

export const SVG_NS = 'http://www.w3.org/2000/svg';
export const XHTML_NS = 'http://www.w3.org/1999/xhtml';

export const MAIN_Y = 240;
export const SPINE_START_X = 140;
export const SPINE_STEP_X = 156;
export const LANE_BASE_OFFSET = 85;
export const LANE_STEP_Y = 75;
// Birleşmiş (kapalı) yayların şerit yüksekliği: tek tanım, yOfLane ve geometri ikisi de bunu kullanır
export const MERGED_LANE_BASE = 52;
export const MERGED_LANE_STEP = 36;
export const FO_W = 222;
export const FO_H = 130; // açık kapsül yüksekliği (Uzak / Yerel satırı dahil)
// Üst yarı-küredeki açık kapsül, yayın tepe noktasının (apex) bu kadar AŞAĞISINA taşar: kartın alt kenarı = apexY + CARD_APEX_OVERHANG.
// (capsule.js yerleşimi ve scene.js hedef çizgisi aynı sabiti kullanır; ikisi ayrışırsa çizgi kartın arkasında kalır.)
export const CARD_APEX_OVERHANG = 12;
export const MERGED_FO_W = 188;
export const MERGED_FO_H = 34;
export const OPEN_CAPSULE_MIN_GAP_X = 238;
export const REPO_STORAGE_KEY = 'gitarc_repo_id';
