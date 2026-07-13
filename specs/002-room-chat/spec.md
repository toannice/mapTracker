# Feature Specification: Room Chat

**Feature Branch**: `002-room-chat`  
**Created**: 2026-05-21  
**Status**: Draft  
**Input**: In-game shared chat room for all players (terminal + Android), Vietnamese support, timestamped history

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Gửi và nhận tin nhắn trong phòng (Priority: P1)

Khi đang trong phòng (lobby hoặc mid-game), người chơi gõ tin nhắn và tất cả người trong phòng nhận được ngay lập tức, kèm tên người gửi và thời gian.

**Why this priority**: Đây là core value của tính năng. Không có khả năng gửi/nhận, chat không tồn tại.

**Independent Test**: Mở 2 client cùng phòng, một bên gửi "Xin chào!", bên kia thấy `[12:34] Alice: Xin chào!` trong vòng 1 giây.

**Acceptance Scenarios**:

1. **Given** hai người trong cùng phòng, **When** Alice gõ "Gg!" và gửi, **Then** Bob thấy `[HH:MM] Alice: Gg!` xuất hiện trong chat log trong vòng 1 giây.
2. **Given** người chơi gõ 201 ký tự, **When** cố gắng gửi, **Then** tin nhắn bị từ chối (hoặc cắt ở client trước khi gửi) với thông báo rõ ràng.
3. **Given** tin nhắn chứa ký tự tiếng Việt (dấu, đặc biệt), **When** gửi và nhận, **Then** nội dung giữ nguyên chính xác.

---

### User Story 2 — Xem lịch sử chat khi mới join phòng (Priority: P2)

Người chơi vào phòng muộn hơn vẫn thấy được cuộc trò chuyện diễn ra trước đó (tối đa 50 tin gần nhất).

**Why this priority**: Không có lịch sử, người join sau bị mất ngữ cảnh hoàn toàn — ảnh hưởng đến trải nghiệm cộng tác.

**Independent Test**: Tạo phòng, gửi 5 tin nhắn, sau đó join thêm client thứ 2 — client thứ 2 thấy đủ 5 tin cũ.

**Acceptance Scenarios**:

1. **Given** phòng có sẵn 10 tin nhắn, **When** người chơi mới join, **Then** welcome payload chứa 10 tin đó (timestamp, tên, nội dung) theo thứ tự thời gian tăng dần.
2. **Given** phòng có 60 tin nhắn, **When** người mới join, **Then** chỉ 50 tin gần nhất được gửi.
3. **Given** phòng chưa có tin nào, **When** người mới join, **Then** chat log rỗng, không có lỗi.

---

### User Story 3 — Chat trên terminal PC (play.ps1 / play.py) (Priority: P3)

Người chơi terminal gõ T để vào chế độ chat, nhập tin, Enter để gửi, Esc để hủy — không xung đột với phím điều khiển game.

**Why this priority**: Terminal là client cần UX thoát ra từ single-key input loop. Đây là adaptation platform-specific, không phải core protocol.

**Independent Test**: Chạy play.ps1, gõ T → prompt `Chat> ` xuất hiện, gõ "hello" + Enter → tin được gửi và xuất hiện trong log của tất cả người chơi.

**Acceptance Scenarios**:

1. **Given** game đang chạy, **When** gõ T, **Then** terminal hiển thị prompt `Chat> ` và chờ input văn bản.
2. **Given** đang ở chế độ chat, **When** gõ Esc (hoặc Ctrl+C), **Then** hủy input, trở về chế độ game bình thường, không gửi gì.
3. **Given** đang ở chế độ chat, **When** gõ text + Enter, **Then** tin được gửi, prompt biến mất, chế độ game khôi phục.
4. **Given** chat_msg đến, **When** đang ở chế độ game bình thường, **Then** in `[HH:MM] Name: message` vào log ngay.

---

### User Story 4 — Chat trên Android app (Priority: P3)

App Android có khu vực chat rõ ràng: game log phía trên, chat log + input box phía dưới. Người chơi gõ và nhấn Send.

**Why this priority**: Quan trọng với UX mobile nhưng là platform adaptation, song hành với User Story 3.

**Independent Test**: Cài APK, join phòng, nhìn thấy layout 2 khu vực, gõ text vào input box → nhấn Send → tin xuất hiện trong chat log của cả Android và terminal.

**Acceptance Scenarios**:

1. **Given** app đã join phòng, **When** màn hình game hiển thị, **Then** game log chiếm khoảng 60% trên, chat log + input box chiếm 40% dưới.
2. **Given** người dùng gõ vào chat input, **When** nhấn Send, **Then** tin được gửi, input box tự xóa, tin xuất hiện trong chat log.
3. **Given** chat_msg đến, **When** app đang foreground, **Then** tin nhắn tự cuộn xuống cuối chat log.
4. **Given** người dùng gõ >200 ký tự, **When** cố gắng gửi, **Then** tin bị từ chối hoặc input bị giới hạn ở 200 ký tự.

---

### Edge Cases

- Người gửi tin nhắn rỗng (chỉ khoảng trắng) → bị từ chối, không broadcast.
- Phòng chỉ có 1 người → vẫn hoạt động (tự thấy tin của mình).
- Người gửi bị disconnect ngay sau khi gửi → tin đã được broadcast, lịch sử vẫn lưu.
- Server restart → lịch sử chat trong phòng mất (in-memory, chấp nhận được — phòng cũng mất).
- Emoji và ký tự Unicode ngoài BMP → xử lý như UTF-8 thông thường, không cần hỗ trợ đặc biệt.
- Nhiều tin nhắn gửi đồng thời → server serialise theo thứ tự nhận, timestamp gán phía server.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Server MUST broadcast mỗi tin nhắn hợp lệ đến tất cả client đang kết nối trong cùng phòng trong vòng 1 giây.
- **FR-002**: Server MUST gán timestamp (epoch milliseconds, phía server) cho mỗi tin nhắn trước khi broadcast.
- **FR-003**: Server MUST lưu tối đa 50 tin nhắn gần nhất trong bộ nhớ phòng; tin cũ hơn bị xóa tự động (FIFO).
- **FR-004**: Server MUST gửi lịch sử chat (≤50 tin) cho mọi client ngay khi join phòng (trong welcome payload hoặc message riêng).
- **FR-005**: Server MUST từ chối tin nhắn rỗng (sau khi trim whitespace) và không broadcast.
- **FR-006**: Server MUST từ chối tin nhắn dài hơn 200 ký tự Unicode và phản hồi lỗi cho người gửi.
- **FR-007**: Client terminal (play.ps1 và play.py) MUST hỗ trợ phím T để vào chế độ nhập chat và Esc để hủy.
- **FR-008**: Client Android MUST hiển thị chat log có thể cuộn (mới nhất dưới cùng) và ô nhập text kèm nút Send.
- **FR-009**: Mọi client MUST hiển thị tin nhắn theo định dạng `[HH:MM] TênNgườiGửi: nội dung`.
- **FR-010**: Nội dung tin nhắn MUST được truyền và lưu dưới dạng UTF-8, hỗ trợ đầy đủ tiếng Việt.
- **FR-011**: Wire protocol: client gửi message type `"chat"`, server broadcast message type `"chat_msg"`.
- **FR-012**: `chat_msg` MUST chứa: `senderName` (string), `ts` (epoch ms), `text` (string).

### Key Entities

- **ChatMessage**: Đại diện một tin nhắn chat. Thuộc tính: `senderName` (tên người gửi), `ts` (epoch milliseconds, gán bởi server), `text` (nội dung, ≤200 ký tự Unicode).
- **ChatHistory**: Danh sách có thứ tự tối đa 50 ChatMessage gần nhất trong một phòng, volatile (mất khi phòng đóng).

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Tin nhắn xuất hiện ở tất cả client trong cùng phòng trong vòng 1 giây sau khi gửi (điều kiện mạng bình thường).
- **SC-002**: 100% ký tự tiếng Việt (kể cả tổ hợp dấu phức tạp) được bảo toàn sau khi truyền và hiển thị.
- **SC-003**: Người join phòng muộn thấy đủ lịch sử chat (≤50 tin) mà không cần refresh thủ công.
- **SC-004**: Không có tin nhắn nào bị dropped khi ≤8 người gửi đồng thời trong cùng phòng.
- **SC-005**: Người chơi terminal có thể gửi tin nhắn mà không bị gián đoạn hoặc nhầm lẫn với phím điều khiển game (W/A/S/D/G/M/F/Q).
- **SC-006**: Chat log Android tự cuộn xuống cuối mỗi khi có tin mới, không cần thao tác thủ công.

---

## Assumptions

- Chat chỉ trong phạm vi phòng (room-scoped); không có chat toàn server hay private message.
- Lịch sử chat là in-memory — mất khi server restart hoặc phòng đóng (không cần persistence).
- Tên người gửi lấy từ tên player đã đăng ký khi join phòng (không cần thêm display name riêng).
- Timestamp định dạng `HH:MM` đủ cho UX (không hiển thị ngày, giây).
- Giới hạn 200 ký tự tính theo Unicode code points, không phải bytes.
- Keypad điều khiển game trên Android (nếu có overlay) vẫn giữ nguyên ở vị trí hiện tại; chat input nằm tách biệt phía dưới.
- Phím điều khiển game trên terminal (W/A/S/D/G/M/F/Q) KHÔNG bị ảnh hưởng — T chỉ kích hoạt khi không ở chế độ shoot-pending.
- Server-side validation là đủ; client-side validation là UX enhancement (không bắt buộc).
- Chat hoạt động cả ở lobby phase lẫn active game phase.
