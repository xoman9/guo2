# 綠果鑑 0.2.536 — 搜尋與撩果移植核對表

來源：使用者提供的 Android ARM64 APK，`lib/arm64-v8a/libduanju_core.so`（Go 原生共享庫，stripped）。此表只記錄直接從 APK 靜態分析確認的資訊，**不把字串當作網路請求成功的證據**。

## 紅果搜尋

Go `.gopclntab` 可還原的函式位置（虛擬位址，非原始碼）：

- `(*Downloader).hongguoAppSearchPage`: `0x344bfb0–0x344c720`
- `(*Downloader).hongguoAppSearchCue`: `0x344c720–0x344cb20`
- `(*Downloader).hongguoAppSearchFallback`: `0x346ede0–0x346f060`

可見路由：

- `/reading/bookapi/search/page/v/`
- `/reading/bookapi/search/cue/v1/`

已在 guo2 接入同名路由及通用 JSON 解析，**並非原函式的位元組等效移植**。原 APK 完整參數、HTTP 簽章方式、實際巢狀資料欄位及錯誤處理仍未驗證。若不具備可核對的網路請求／回傳樣本，不應聲稱已修復搜尋。新版與舊版均失敗時保留名稱匹配結果，並顯示失敗狀態。

## 撩果

APK 內可確認的字串：

- `liaoguo`
- `liaoguo-jingpin`, `liaoguo-guochan`, `liaoguo-zipai`, `liaoguo-zimu`
- `aeg.xslrxx.mom`、`xslrxx.mom`、`xslrxx.rest` 等

Go `.gopclntab` 中可還原的共用函式：

- `(*Downloader).fetchMaccmsCatalogPage`: `0x3403730–0x34051f0`

這並不直接證明撩果所有分類都使用該函式。舊版 guo 原始碼把 `liaoguo` 登記為 `maccms`，但此分類是歷史來源，不是從新版 APK 的控制流程驗證得到的。guo2 目前共用的數字分類 ID 與新版 APK 的文字分類 key **不能假定互相對應**。不能以網站首頁取得影片，便宣稱與 APK 相同。

## 下一步的可驗證依據

- APK 在使用者裝置正常搜尋與點選撩果各分類時，經同意取得的請求路徑、參數（遮蔽憑證、Cookie、裝置識別碼）及去識別回應欄位結構。
- 不使用固定 Cookie、猜測驗證 token、或偽稱能繞過站點瀏覽器驗證。
- 在各自站源中以相同影片 ID 比較列表、分類、詳情及播放資料；不同則視為移植未完成。
- 保留 ARM64 與 armeabi-v7a，正式結果以 GitHub Actions + 實機測試為準。
