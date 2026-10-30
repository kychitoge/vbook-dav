## Mô Tả Thay Đổi (Description)
<!-- Tóm tắt ngắn gọn lý do và nội dung các thay đổi trong PR này -->

## Loại Thay Đổi (Type of Change)
- [ ] Sửa lỗi (Bug fix)
- [ ] Tính năng mới (New feature)
- [ ] Cải thiện tài liệu (Documentation update)
- [ ] Tái cấu trúc mã nguồn (Refactoring / Performance)

## Checklist Thẩm Định Kỹ Thuật (Pre-Merge Checklist)
- [ ] Đã chạy kiểm thử tuần tự thành công: `go test -count=1 -p 1 ./...`
- [ ] Đã chạy phân tích tĩnh: `go vet ./...` và định dạng `go fmt ./...`
- [ ] Đã biên dịch thử nghiệm thành công: `go build ./cmd/dav`
- [ ] Tuân thủ nguyên tắc Dao cạo Occam (Không thêm external database, không đưa dependency thừa thãi)
- [ ] Không phá vỡ 4 dị biệt bắt buộc của vBook (Idempotent MKCOL, Auto-MkdirAll, Bypass OPTIONS, Telemetry)
- [ ] Nếu sửa đổi tài liệu người dùng, đảm bảo định dạng chuẩn song ngữ (nửa trên Tiếng Việt, nửa dưới Tiếng Anh)
