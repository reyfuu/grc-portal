# GRC Portal

Portal internal GRC/compliance dengan tiga modul use case:
- **User Access Review (UAR)** — model IAM (application → role → subject → binding) + rekonsiliasi akses ke database HR (tandai akun nonaktif/mismatch) + campaign certification (reviewer certify/revoke tiap akses per periode).
- **SOP & Kebijakan** — pencarian dokumen internal + jawaban AI (RAG) dengan referensi.
- **Evidence Management** — request evidence audit/assessment dicocokkan ke evidence yang pernah dipakai untuk reuse (jejak sumber) atau ditandai gap + diarahkan ke owner, dengan human approval.

Fondasi bersama: auth (session cookie + bcrypt), applications, audit log, layer db, dan frontend Vue yang di-embed ke binary Go.

## Stack
- Backend: Go (stdlib `net/http`) + PostgreSQL (pgx via `database/sql`)
- Frontend: Vue 3 + Vite + Vue Router (di-embed ke binary Go)
- Auth: session cookie (HttpOnly, SameSite=Lax), bcrypt

## Prasyarat
- Go 1.22+ (dites di 1.26)
- Node 18+ / npm
- PostgreSQL, dan sebuah database kosong

## Jalankan (lokal)
```bash
createdb grc   # sekali saja

# build frontend + binary
make build

# muat kunci AI supaya tombol "Tanya AI" (modul SOP) berfungsi — tanpa ini jawaban AI mati
set -a; . ~/api/.env; set +a   # GOOGLE_API_KEY / OPENAI_API_KEY

# jalankan (seed admin pertama dari env, hanya saat belum ada user)
DEV=1 \
ADMIN_EMAIL=admin@grc.local ADMIN_PASSWORD=admin123 \
DATABASE_URL='postgres://localhost:5432/grc?sslmode=disable' \
./bin/grcportal
# buka http://localhost:8081 — login admin@grc.local / admin123
```
> **Tombol "Tanya AI" tidak menjawab?** Kalau hanya muncul dokumen (tanpa jawaban AI),
> berarti key AI belum ter-load. Pastikan `GOOGLE_API_KEY` (atau `OPENAI_API_KEY`) ada di
> environment saat binary dijalankan — jalankan `set -a; . ~/api/.env; set +a` lebih dulu.
`DEV=1` mematikan flag `Secure` cookie supaya login jalan di http lokal. Di produksi (HTTPS) jangan set `DEV`.

## Dev loop (hot reload)
```bash
# terminal 1
set -a; . ~/api/.env; set +a   # kunci AI untuk "Tanya AI"
DEV=1 DATABASE_URL='postgres://localhost:5432/grc?sslmode=disable' make dev-api   # :8081
# terminal 2
make dev-web    # :5173, proxy /api -> :8081  → buka http://localhost:5173
```

## Test
```bash
go test ./...   # logika decide review + validasi CSV (tanpa perlu DB)
```

## Env
| Var | Default | Keterangan |
|-----|---------|-----------|
| `DATABASE_URL` | `postgres://localhost:5432/grc?sslmode=disable` | koneksi Postgres |
| `ADDR` | `:8081` | alamat listen (8080 sering dipakai Apache lokal) |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | — | seed admin pertama (sekali) |
| `DEV` | — | jika di-set, cookie tanpa flag Secure (untuk http lokal) |
| `GOOGLE_API_KEY` / `GEMINI_API_KEY` | — | **diprioritaskan** bila diset — jawaban AI pakai Gemini (endpoint OpenAI-compatible) |
| `OPENAI_API_KEY` | — | fallback: router/OpenAI-compatible lain untuk jawaban AI |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | endpoint OpenAI-compatible (mis. router internal) |
| `AI_MODEL` | `gemini-2.5-flash` (Gemini) / `cx/gpt-5.4-mini` (OpenAI) | model chat untuk sintesis jawaban RAG |

## Modul SOP & Kebijakan (pencarian AI / RAG)
Menu **SOP & Kebijakan** (`/sop`): simpan dokumen SOP/kebijakan/evidence, lalu tanya
dalam bahasa natural. Retrieval pakai **full-text search Postgres** (lokal, tanpa
embedding) → dokumen relevan diberikan ke chat model (Gemini / OpenAI-compatible) untuk
jawaban ringkas + referensi. Provider: set `GOOGLE_API_KEY` (atau `GEMINI_API_KEY`) untuk Gemini — dipakai
duluan — atau `OPENAI_API_KEY`. Tanpa keduanya, menu tetap jalan sebagai pencarian
dokumen biasa (jawaban AI dinonaktifkan). Jalankan dengan env AI di-load:
```bash
set -a; . ~/api/.env; set +a   # GOOGLE_API_KEY (atau OPENAI_API_KEY + OPENAI_BASE_URL)
DEV=1 DATABASE_URL='postgres://localhost:5432/grc?sslmode=disable' ./bin/grcportal
```

## Access Review — rekonsiliasi vs HR (`/access-review`)
Konsolidasikan subject yang punya akses aktif, cocokkan ke **database HR** (by email,
fallback username), lalu tandai **daftar konfirmasi**: akun `inactive` (nonaktif di HR
tapi akses masih hidup) dan `not_in_hr` (punya akses tapi tak ada di HR / mismatch).
Service account dikecualikan. Ini hanya menyiapkan daftar — keputusan cabut/keep tetap
lewat approval di **Campaigns**.

Isi database HR: tambah manual di menu, atau import CSV (admin) — header wajib:
```
email,username,display_name,department,status
```
`status` = `active` | `inactive`. Upsert by email (fallback username), idempotent.
API: `GET /api/access-review` (`?flagged=1` untuk hanya yang bermasalah), `GET/POST/DELETE /api/hr-employees`, `POST /api/import/hr`.

## Modul Evidence Management (`/evidence`)
Request evidence berulang dari auditor/assessor sering minta bukti yang sama/mirip.
Menu **Evidence** mencatat request, lalu **menyarankan evidence yang pernah dipakai**
(FTS atas dokumen kategori `evidence` — memakai ulang retrieval modul SOP) dan
**mendeteksi request serupa** (dedup). Saran bersifat advisory; reviewer/admin yang
memutuskan: `fulfilled` (tautkan dokumen = jejak sumber) atau `gap` (arahkan ke owner,
mis. Infrastructure/Developer). Setiap keputusan tercatat di audit log.

API: `GET/POST /api/evidence-requests`, `GET /api/evidence-requests/{id}` (request +
`matches` + `similar`), `POST /api/evidence-requests/{id}/resolve` (`{action, document_id|owner, note}`).

## Import CSV (bulk akses)
Header wajib:
```
application,subject_email,subject_username,subject_display_name,subject_type,role,granted_at
```
Aplikasi harus dibuat lebih dulu; baris dengan aplikasi tak dikenal ditolak dengan nomor barisnya. Import bersifat idempotent (binding duplikat dilewati).

## Deploy
`make build` menghasilkan satu binary (`bin/grcportal`) yang sudah memuat frontend. Butuh instance PostgreSQL (mis. managed: Neon/Supabase, atau self-host) — set `DATABASE_URL`. Pastikan diserve via HTTPS agar cookie `Secure` terkirim.
