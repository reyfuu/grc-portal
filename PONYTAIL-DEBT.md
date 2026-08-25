# Ponytail Debt Ledger

Deliberate shortcuts marked with `ponytail:` comments, collected so a deferral
can't quietly become permanent. Regenerate with `/ponytail-debt`.

## internal/db/schema.sql

- **schema.sql:2** — skema dibuat lewat `CREATE TABLE IF NOT EXISTS` saat boot (bukan migrasi bernomor).
  - ceiling: hanya aman untuk menambah tabel/kolom secara idempoten.
  - upgrade: saat butuh `ALTER` yang mengubah/menghapus kolom → tambah tabel `schema_migrations` + file `.sql` bernomor.
- **schema.sql:129** — `evidence_requests` menautkan satu evidence per request (`document_id`).
  - ceiling: satu jejak sumber per request.
  - upgrade: kalau butuh banyak evidence per request → tambah tabel join `evidence_request_documents`.

---

**2 markers, 0 with no trigger.**
