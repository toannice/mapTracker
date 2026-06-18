# Event Broadcast Reference

Tất cả event được phát đi qua `broadcastTurnResult` → `BuildPlayerView` cho **từng** player đang kết nối.  
`BuildPlayerView` áp hai lớp lọc trước khi đưa event vào `PlayerView.events`:

| Lớp lọc | Điều kiện | Hành vi |
|---|---|---|
| **InfoBlackout** | `p.InfoBlackout == true` khi nhận turn_result | Toàn bộ event bị thay bằng 1 event `blackout_info` duy nhất (chỉ tên người hành động, không có gì khác). Flag reset ngay sau đó. |
| **Private** (`ForPlayerID`) | `ForPlayerID != ""` | Chỉ player có ID khớp mới thấy event. Mọi người khác không nhận được. |
| **Actor-strip** (`ActingPlayerID`) | `ActingPlayerID != ""`, người nhận ≠ actor | Event vẫn được gửi nhưng field `bulletFull` bị xoá khỏi payload. |

> **Quy ước cột "Ai nhận"** trong bảng dưới:  
> - **Tất cả** = mọi player trong phòng (sau lọc InfoBlackout)  
> - **Chỉ trigger** = chỉ player gây ra event (`ForPlayerID = trigger`)  
> - **Actor-strip** = tất cả nhận nhưng trigger nhận payload đầy đủ, người khác bị strip field

---

## 1. player_moved

Phát mỗi khi một player thực hiện action `move`, kể cả khi di chuyển thất bại (va tường / OOB).

| Trường hợp | `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|---|
| Di chuyển thành công | — | **trigger** | **Tất cả** (actor-strip) | `playerName`, `direction` ("up"/"down"/"left"/"right"), `success: true`, `blockType` ("blank"/"bullet"/"reward"/"trap"/"info"/"portal"), `bulletFull: bool` |
| Di chuyển thất bại (tường/OOB) | — | — | **Tất cả** | `playerName`, `direction`, `success: false` |

> `bulletFull` chỉ có ý nghĩa với chính trigger (đã có bullet rồi → không nhặt thêm được). Observer không cần biết.

---

## 2. trap_triggered

Phát khi player bước vào ô trap hoặc bị teleport vào ô trap (chain).  
**Luôn gửi cho tất cả** — không có flag private. Observer biết có bẫy nhưng không biết vị trí mới (nếu teleport).

### 2a. reveal_position (15%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "reveal_position"`, `playerId`, `playerName`, `pos: {x,y}` |

### 2b. random_teleport (20%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "random_teleport"`, `playerName` |

> Vị trí mới **không** được broadcast. Observer chỉ biết "ai đó bị teleport". Nếu sau teleport trigger vào ô đặc biệt khác, event của ô đó tiếp tục được emit ngay sau.

### 2c. lose_next_turn (40%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "lose_next_turn"`, `playerName` |

### 2d. lose_bullet (15%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "lose_bullet"`, `playerName`, `lost: bool` |

> `lost: false` nếu player không có bullet nào để mất.

### 2e. info_blackout (10%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "info_blackout"`, `playerName` |

> Lượt **tiếp theo** của player đó, `BuildPlayerView` sẽ bọc mọi event trong `blackout_info`. Observer thấy "ai đó bị blackout" nhưng lượt sau họ không bị ảnh hưởng gì.

---

## 3. reward_activated

Phát khi player bước vào ô reward.

### 3a. all_positions_revealed (20%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "all_positions_revealed"`, `playerName`, `positions: [{name, pos}]` |

> Đây là sự kiện duy nhất tiết lộ vị trí của **mọi player** cho **mọi người** cùng lúc.

### 3b. nuke_pending (40%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "nuke_pending"`, `playerName`, `nukeSide: int` |

> Game tạm dừng (`Paused = true`). Trigger phải gửi action `nuke_target` để chọn vị trí. Mọi người trong phòng thấy game bị pause và biết ai đang giữ nuke.

### 3c. all_bullet_locations (40%)
| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `effect: "all_bullet_locations"`, `playerName`, `locations: [{x,y}]` |

> Vị trí tất cả ô bullet trên map tại thời điểm hiện tại được tiết lộ cho cả phòng.

---

## 4. clue_received

Phát khi player bước vào ô info (compass). **Luôn private** — `ForPlayerID` luôn được set bằng ID của trigger.

| Clue type | `ForPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| `nearest_direction` (40%) | **trigger** | **Chỉ trigger** | `type: "nearest_direction"`, `direction: "up"/"down"/"left"/"right"/"none"` |
| `own_start_pos` (20%) | **trigger** | **Chỉ trigger** | `type: "own_start_pos"`, `pos: {x,y}` |
| `other_player_pos` (20%) | **trigger** | **Chỉ trigger** | `type: "other_player_pos"`, `playerName`, `pos: {x,y}` |
| `surroundings_3x3` (20%) | **trigger** | **Chỉ trigger** | `type: "surroundings_3x3"`, `cells: [{pos,kind}]` (tối đa 8 ô xung quanh) |

> Nếu không có player nào khác còn sống, `other_player_pos` fallback về `nearest_direction`.  
> Observer không nhận được event này — họ chỉ thấy `player_moved` với `blockType: "info"`.

---

## 5. shot_fired

Phát khi player thực hiện action `shoot` (có bullet).

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `byPlayerId`, `byPlayerName`, `direction` |

> Hướng đạn luôn được công bố. Ai bị bắn sẽ nhận thêm event `player_eliminated` ngay sau.

---

## 6. player_eliminated

Phát ngay sau `shot_fired` nếu đạn trúng người, hoặc sau `nuke_fired` nếu player trong vùng nổ.

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `playerName` (người bị loại), `byPlayerName` (người bắn/nuke) |

> Nếu chỉ còn 1 người sống sót sau event này, `state.Phase` chuyển sang `ended` và `game_over` được broadcast ngay trong cùng lượt.

---

## 7. portal_used

Phát khi player bước vào ô portal (A hoặc B) và được dịch chuyển sang portal đối diện.

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `dest: {x,y}` |

> Điểm đến được tiết lộ công khai. Nếu ô đích có effect (trap/reward/info), event của ô đó emit tiếp ngay sau nhưng **không** theo portal thêm lần nữa (giới hạn 1 portal mỗi lượt).

---

## 8. nuke_fired

Phát khi trigger gửi action `nuke_target` để kích hoạt nuke đang pending.

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `byPlayerName`, `topX`, `topY`, `side`, `eliminated: [playerName]` |

> `player_eliminated` event riêng lẻ vẫn được emit thêm cho mỗi người bị nuke (xem mục 6).

---

## 9. map_submitted

Phát khi player gửi action `submit_map`.

| Trường hợp | `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|---|
| Đúng hoàn toàn | — | — | **Tất cả** | `playerName`, `correct: true`, `wrong: 0` |
| Sai một phần | — | — | **Tất cả** | `playerName`, `correct: false`, `wrong: int` (số ô sai, tính symmetric diff), `submitsLeft: int` |

> Submit không tốn lượt. Game không dừng khi hết `submitsLeft`; player chỉ nhận lỗi `NO_SUBMIT_LEFT` ở lần tiếp theo.

---

## 10. turn_skipped

Phát khi lượt của một player bị bỏ qua: hết giờ, mất kết nối, hoặc đang bị `SkipNextTurn`.

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Tất cả** | `playerId`, `playerName` |

---

## 11. blackout_info

**Không phải event thật** — được tạo ra bởi `BuildPlayerView` thay thế mọi event khác khi người nhận đang trong trạng thái `InfoBlackout`.

| `ForPlayerID` | `ActingPlayerID` | Ai nhận | Payload |
|---|---|---|---|
| — | — | **Chỉ player bị blackout** | `playerName` (tên người đã hành động) |

> Người bị blackout biết "ai đó vừa làm gì đó" nhưng không biết là gì. Flag tự reset sau khi `BuildPlayerView` chạy xong.

---

## Tổng hợp nhanh

| Event | Ai trigger | Ai nhận | Có thông tin vị trí? |
|---|---|---|---|
| `player_moved` (success) | Actor | Tất cả (strip `bulletFull`) | Không (chỉ hướng) |
| `player_moved` (fail) | Actor | Tất cả | Không |
| `trap_triggered / reveal_position` | Bước vào trap | Tất cả | ✓ Vị trí của trigger |
| `trap_triggered / random_teleport` | Bước vào trap | Tất cả | ✗ |
| `trap_triggered / lose_next_turn` | Bước vào trap | Tất cả | ✗ |
| `trap_triggered / lose_bullet` | Bước vào trap | Tất cả | ✗ |
| `trap_triggered / info_blackout` | Bước vào trap | Tất cả | ✗ |
| `reward_activated / all_positions_revealed` | Bước vào reward | Tất cả | ✓ Vị trí **tất cả** player |
| `reward_activated / nuke_pending` | Bước vào reward | Tất cả | ✗ |
| `reward_activated / all_bullet_locations` | Bước vào reward | Tất cả | ✓ Vị trí tất cả ô bullet |
| `clue_received / nearest_direction` | Bước vào info | **Chỉ trigger** | ✗ |
| `clue_received / own_start_pos` | Bước vào info | **Chỉ trigger** | ✓ Vị trí start của trigger |
| `clue_received / other_player_pos` | Bước vào info | **Chỉ trigger** | ✓ Vị trí 1 player khác |
| `clue_received / surroundings_3x3` | Bước vào info | **Chỉ trigger** | ✓ 8 ô xung quanh |
| `shot_fired` | Bắn | Tất cả | ✗ |
| `player_eliminated` | Bị trúng đạn/nuke | Tất cả | ✗ |
| `portal_used` | Bước vào portal | Tất cả | ✓ Vị trí đích |
| `nuke_fired` | Chọn `nuke_target` | Tất cả | ✓ Vùng nổ (topX, topY, side) |
| `map_submitted` | Submit bản đồ | Tất cả | ✗ |
| `turn_skipped` | Hết giờ / skip | Tất cả | ✗ |
| `blackout_info` | — (virtual) | Chỉ player bị blackout | ✗ |
