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


## 2026-11-11: ARM64 反組譯已證實的呼叫鏈

分析來源：使用者上傳的 `绿果鉴-0.2.536-unlocked【TG@yydsys】.apk`，其 `lib/arm64-v8a/libduanju_core.so` 的 `.gopclntab` 函式表、`go tool addr2line` 及 AArch64 `bl` 呼叫目標。

### 紅果搜尋（已核實的直接呼叫）

- `hongguoAppSearchPage`（`provider_hongguo_app_features.go:53`, `0x344bfb0`）：直接呼叫 `hongguoAppModernRequestTo`（`provider_hongguo_app.go:156`）及 `hongguoAppCollectRows`（`provider_hongguo_app_features.go:31`）。
- `hongguoAppSearchCue`（`provider_hongguo_app_features.go:97`, `0x344c720`）：直接呼叫 `hongguoAppModernRequestTo`。
- `hongguoAppModernRequestTo` 直接呼叫 `hongguoAppRequestToMode`（`provider_hongguo_app.go:216`）。後者直接呼叫 `hongguoAppQuery`、`http.NewRequestWithContext`、`doCatalogRequestWithClient`、`rotateHongguoDevice`、`json.Decoder.Decode`、`hongguoBusinessCode`。
- `hongguoAppSearchFallback`（`provider_hongguo_search.go:368`, `0x346ede0`）直接呼叫 `fetchHongguoAppCatalogCategory`（`provider_hongguo_catalog.go:126`），不是只重試舊版 `search/tab`。
- 因此目前 guo2 的新版路由 + `hongguoAppRequest` + 通用 JSON 解析 + 舊版 tab 備援**不等同於 APK 的搜尋實作**。需要核對 RequestToMode 的參數、模式、headers、使用的 client、解析 CollectRows 的欄位、分類備援策略。尚未確認新版搜尋 API 是否能在實機返回有效結果。

### MacCMS / 撩果驗證（已核實的直接呼叫）

- `fetchMaccmsCatalogPage`（`provider_duanju_maccms.go:332`, `0x3403730`）直接呼叫 `duanjuBaseURL`、`duanjuSourceAgent`、`fetchProviderPage`、`maccmsCards`、`followMaccmsChallenge`。
- `followMaccmsChallenge`（`provider_duanju_maccms.go:304`, `0x3403530`）直接呼叫 `fetchProviderText`、`regexp.FindStringSubmatch`、`duanjuAbsolute`、`isProviderHTTPMediaURL`、`html.ParseWithOptions`。這表明有跟隨／處理驗證相關網頁的原生流程，但單憑此函式無法證明「自動繞過」或成功通過。
- APK 另有 `challenge.go`、`challenge_runtime.go` 的 `markPending`、`acceptChallenge`、`complete`、`cookieHeader`、`challengeJar.Cookies` 等函式，顯示具備保存驗證工作階段的通用基礎設施。**尚未證實這套 challengeStore 直接由撩果呼叫**。
- `liaoguo-jingpin`、`liaoguo-guochan`、`liaoguo-zipai`、`liaoguo-zimu` 是 APK 中的分類字串；與 guo2 的「數字分類 ID」沒有已驗證的等價映射。
- 外部瀏覽器開啟入口不能保證將 Cookie／User-Agent／Session 狀態帶回原生 Go HTTP 客戶端。不得稱其為已完成的驗證修復。

### 實作前必須取得／還原

1. 對應 APK 的 `hongguoAppRequestToMode` 參數及實際 HTTP 請求方式；確認任何裝置註冊與服務端授權均為合法正式流程。
2. `hongguoAppCollectRows` 處理的實際 response schema、`hongguoAppSearchFallback` 分類匹配策略。
3. `followMaccmsChallenge` 對應的頁面辨識與合法使用者驗證後 Session 保存方法，確認是否確實由撩果使用。
4. 實機比對同關鍵字與分類的影片 ID、片名、封面及分頁。請求／回應樣本不得包含可用 Cookie、授權碼或識別碼。

**結論**：這次完成的是有函式呼叫證據的靜態分析，尚未等效實作。不可再宣稱單純調整超時或入口就是搜尋／撩果修復。


## 2026-11-11：新增指令層級確認（優先於前述推測）

利用 ARM64 指令和 Go `.gopclntab` 還原 `hongguoAppSearchPage`（`0x344bfb0–0x344c720`）的真實參數組裝，確認其直接呼叫 `hongguoAppModernRequestTo` 時：

- HTTP method：`GET`（指令所引用的常數 `0x4a79b9`，長度 3）。
- endpoint：**`/reading/bookapi/search/tab/v`**（常數 `0x4e880e`，長度 29，無結尾 `/`）。
- `query`、`offset`、`count`、`tab_type`、`client_req_type`、`selector` 均被放入查詢參數。
- `tab_type` 常數值為 `29`，`client_req_type` 為 `2`。
- `selector` 是長度 92 的 JSON 字串：`[{"selector_item_id":"content_type_short_play","show_name":"真人剧","value":"真人剧"}]`。
- 回傳結果使用 `search_tabs` 鍵，並呼叫 `hongguoAppCollectRows`，該函式引用 `video_data` 鍵。

**重要更正：先前認為 APK 的 `hongguoAppSearchPage` 以 `/reading/bookapi/search/page/v/` 為主要路由是錯誤的。** `page/v/` 雖是原生庫中的字串，但沒有證明為這個函式所使用。guo2 已將第一優先請求改回經指令確認的 `search/tab/v`，並加上 `selector` 及 `client_req_type`。原 APK 的 `hongguoAppModernRequestTo` 與現有 `hongguoAppRequest` 在裝置註冊、HTTP 簽章或客戶端選擇方面是否完全等同，仍未確認；仍須實際回應或實機測試。

撩果方面，`followMaccmsChallenge` 確實由 `fetchMaccmsCatalogPage` 呼叫，包含 regexp、同站網址處理與 HTML 解析；但未證實它會操作 `challengeStore` 或完成 Cloudflare／互動式驗證。原 APK 亦包含其他站源使用的 `cf_clearance` 字串，**不能直接歸因於撩果**。
