import type { LocationsKey } from "../locations.i18n";

export const locationsStrings: Record<LocationsKey, string> = {
  secLocations: "カスタムロケーション",
  locationsDesc: "パスごとに別のアップストリームへ転送します。各ロケーションにもホストの保護(セキュリティルール、レート制限、Basic 認証、アクセスキュー)が適用されます。ロケーション / は転送先を置き換えます。",
  locationsCount: "{n} 件のロケーション",
  locationOne: "1 件のロケーション",
  addLocation: "ロケーションを追加",
  locPath: "パス",
  locForwardPath: "転送パス",
  locForwardPathHint: "任意。一致したパスをこのパスに置き換えて転送します(例: /api → /v2/)。",
  locAdvanced: "詳細設定(ディレクティブのみ、管理者のみ)",
};
