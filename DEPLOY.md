# Checklist go-live (free tier)

```
Trình duyệt ──> Vercel project "web" (Next.js) ──/api──> Vercel project "api" (Go) ──> Supabase (Postgres)
     └── video HLS <── Cloudflare R2 (media.<domain>) <── worker trên máy bạn (ffmpeg)
```

Tất cả chạy trên gói free, **không cần thẻ**. Đặt function region của cả 2 project Vercel **cùng region với Supabase**:
Tokyo (`ap-northeast-1`) → `hnd1`, Singapore (`ap-southeast-1`) → `sin1`.
Ghi lại mọi giá trị bí mật vào trình quản lý mật khẩu, **không** commit.

## 0. Chuẩn bị code

- [ ] `make test-db` và `cd web && npm run build` đều pass.
- [ ] Thư mục `lumen` chưa phải git repo riêng. Tạo repo **private** trên GitHub rồi:
      `cd lumen && git init && git add . && git commit -m "init" && git remote add origin <url> && git push -u origin main`
- [ ] Trước khi push, kiểm tra `git status` không có `api/.env` hay `web/.env.local` (đã có trong `.gitignore`).
- [ ] Thay chữ "Powered by TMDB" trong `web/components/Nav.tsx` bằng logo TMDB được phép dùng (điều khoản TMDB).

## 1. Domain (khuyến nghị, không bắt buộc)

- [ ] Có domain: `.id.vn` miễn phí nếu 18–23 tuổi, hoặc mua `.com` ở Cloudflare Registrar.
- [ ] Thêm domain vào Cloudflare (Free plan), đổi nameserver ở nhà đăng ký sang Cloudflare, chờ trạng thái **Active**.
- Không có domain: dùng tạm `<tên>.vercel.app` và URL `r2.dev` (r2.dev bị giới hạn tốc độ, chỉ nên dùng để thử).

## 2. TMDB

- [ ] themoviedb.org → Settings → API → copy **API Read Access Token** → `TMDB_TOKEN`.

## 3. Database: Supabase

- [ ] Chọn project cho production:
      - **Khuyến nghị:** tạo project mới cho production (cùng region Tokyo hoặc Singapore), tách khỏi DB dev.
        Free cho phép 2 project đang chạy.
      - Hoặc dùng project hiện tại (Tokyo `ap-northeast-1`): chạy được, nhưng dev và production dùng chung dữ liệu.
- [ ] Connect → **Session pooler** (host `aws-0-<region>.pooler.supabase.com`, **port 5432**) → `DATABASE_URL`.
      - Không dùng *Transaction pooler* (port 6543): không hỗ trợ prepared statements mà pgx dùng.
      - Không dùng *Direct connection* (`db.<ref>.supabase.co`): chỉ có IPv6, Vercel Functions không kết nối được.
      - Bảng tự tạo khi API khởi động lần đầu.
- [ ] Tạo tài khoản admin từ máy bạn (nhập mật khẩu khi được hỏi):
      `cd api && DATABASE_URL='<supabase url>' go run ./cmd/admin -email you@example.com`
- [ ] **Không** chạy `make seed` lên DB production: phim seed không có video.
      Nếu dùng lại project hiện tại mà đã seed: chạy `make seed-remove` trước khi mở site.
- [ ] Free: 500 MB, project **bị tạm dừng nếu 7 ngày ít truy vấn** (xem bước 8).

## 4. Video: Cloudflare R2

- [ ] Tạo bucket `lumen-media` (location hint: Asia-Pacific).
- [ ] Settings → Custom Domains → thêm `media.<domain>` → `MEDIA_BASE_URL=https://media.<domain>`
      (không có domain: bật r2.dev và dùng URL đó).
- [ ] Settings → CORS policy (thay domain thật; giữ localhost nếu còn dev):
      ```json
      [{"AllowedOrigins":["https://<domain>","http://localhost:3000"],
        "AllowedMethods":["GET","HEAD","PUT"],
        "AllowedHeaders":["Content-Type","Range"],
        "ExposeHeaders":["ETag","Content-Length","Content-Range"],
        "MaxAgeSeconds":3600}]
      ```
- [ ] R2 → Manage API tokens → *Object Read & Write*, chỉ cho bucket này → `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`.
- [ ] Free: 10 GB. Một phim ~90 phút đủ 3 độ phân giải tốn ~3–4 GB. Theo dõi dung lượng ở R2 → Metrics.

## 5. API: Vercel (project thứ nhất)

Go runtime của Vercel (đang Beta) tự build `api/cmd/api` và chạy như một HTTP server. Cấu hình nằm ở `api/vercel.json`.

- [ ] Add New → Project → repo → **Root Directory `api`**. Framework Preset tự nhận **Go** (nếu không, chọn Go).
      Đặt tên project, ví dụ `lumen-api` → URL `https://lumen-api.vercel.app`.
- [ ] Settings → Functions → Function Region: `hnd1` hoặc `sin1` (theo region Supabase).
- [ ] Environment Variables (Production):

      | Key | Giá trị |
      |---|---|
      | `DATABASE_URL` | Supabase Session pooler URL (port 5432) |
      | `TMDB_TOKEN` | từ bước 2 |
      | `TMDB_LANGUAGE` / `TMDB_REGION` | `vi-VN` / `VN` |
      | `SESSION_SECRET` | `openssl rand -hex 32` (tạo mới, khác local) |
      | `SITE_ORIGIN` | `https://<domain>` (đúng domain bạn mở /studio, không có `/` cuối) |
      | `COOKIE_SECURE` | `true` |
      | `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` | từ bước 4 |
      | `R2_BUCKET` | `lumen-media` |
      | `MEDIA_BASE_URL` | `https://media.<domain>` |

- [ ] Deploy xong: mở `https://lumen-api.vercel.app/healthz` → `ok`; `/api/browse` trả về JSON.
- [ ] Settings → Deployment Protection: tắt **Vercel Authentication** cho Production nếu nó bật,
      nếu không project web sẽ nhận 401 khi gọi API.

## 6. Web: Vercel (project thứ hai)

- [ ] Add New → Project → repo → Root Directory `web`, framework Next.js.
- [ ] Env: `API_URL=https://lumen-api.vercel.app` (dùng lúc build cho rewrite `/api`; đổi thì phải **Redeploy**).
- [ ] Settings → Functions → Function Region: cùng region với project API.
- [ ] Settings → Domains → thêm `<domain>` (và `www.<domain>` redirect về nó). Trên Cloudflare DNS, bản ghi trỏ về Vercel để **DNS only** (mây xám).
- [ ] Nếu domain cuối khác lúc cấu hình bước 5: sửa `SITE_ORIGIN` ở project API (rồi Redeploy) và CORS của R2 cho khớp.

## 7. Worker (encode) trên máy bạn

- [ ] Tạo file env production tên `api/prod.env.local` (khớp `*.env.local` trong `.gitignore`, không bị commit).
      Nội dung: `DATABASE_URL` (Supabase production), các biến `R2_*`, `MEDIA_BASE_URL`, `TMDB_TOKEN`, `WORK_DIR`, `DELETE_SOURCES=true`.
- [ ] Chạy khi có phim cần encode: `set -a; . <file env>; set +a; cd api && go run ./cmd/worker` (cần `ffmpeg`).

## 8. Giữ DB không bị tạm dừng

- [ ] Không cần làm gì thêm: `api/vercel.json` có cron gọi `/api/browse` mỗi ngày lúc 03:00 UTC, đủ để Supabase
      không tạm dừng project vì 7 ngày ít truy vấn. Kiểm tra ở project API → Settings → Cron Jobs.
      Nếu vẫn bị dừng, vào dashboard Supabase bấm *Restore*.
- Vercel Functions không ngủ 15 phút như Render. Sau lúc rảnh chỉ có cold start ngắn (khởi động Go binary và mở kết nối DB).

## 9. Kiểm tra sau khi lên

- [ ] Trang chủ, `/browse` (có nút Previous/Next khi >48 phim), tìm kiếm, trang phim đều hiển thị.
- [ ] `/studio`: đăng nhập, tìm TMDB, upload một clip ngắn → trạng thái *Queued* → worker encode → *Ready* → tick **Published**.
- [ ] Phim phát được trên máy tính và điện thoại; phụ đề `.vtt` hiện đúng.
- [ ] DevTools → Network: file `.m3u8`/`.ts` tải từ `media.<domain>`, không lỗi CORS.
- [ ] Sai mật khẩu 5 lần → bị khoá 15 phút (429). Bộ đếm nằm trong bộ nhớ từng instance, nên trên Vercel
      nó có thể reset khi instance đổi. Vì vậy nên làm thêm bước 10.

## 10. Bảo mật thêm (khuyến nghị)

- [ ] Cloudflare Zero Trust → Access → bảo vệ `/studio*` và `/api/admin*` bằng email của bạn (free ≤ 50 user).
      Access chỉ chặn được khi bản ghi DNS bật proxy (mây cam), trái với cấu hình DNS only ở bước 6;
      Vercel vẫn chạy sau proxy nhưng không khuyến khích. Làm sau khi site đã ổn định.
- [ ] Đổi `SESSION_SECRET` ở project API (rồi Redeploy) để đăng xuất mọi phiên khi cần.
