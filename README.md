# Omyang - Termux AI Agent Harness (Spartan Edition)

Dalam mitologi Jawa, terdapat legenda tentang Keris Omyang (atau Omyang Jimbe). Keris ini dipercaya bukan sekadar senjata tajam biasa, melainkan sebuah pusaka sakti yang memiliki "khodam" atau kekuatan spiritual di dalamnya. Sebuah keris biasa hanya bisa digunakan untuk memotong, namun ketika diisi oleh Omyang, ia berubah menjadi pusaka yang mampu menjaga, membawa kelimpahan, dan bekerja secara mandiri untuk tuannya.

Filosofi inilah yang menjadi nyawa dari project ini. Anggap saja perangkat Android dan aplikasi Termux kamu adalah bilah kerisnya. Di tangan orang biasa, Termux hanyalah layar hitam dengan teks kaku. Namun, ketika "diisi" dengan **Omyang Agent**, Android kamu berubah menjadi *pusaka digital*—sebuah mesin DevOps, asisten sistem admin, dan eksekutor skrip otomatis yang cerdas, mandiri, dan sangat ringan.

Omyang adalah agen eksekutor AI berbasis Command Line Interface (CLI) murni yang dirancang khusus untuk berjalan secara native di Termux Android (atau terminal Linux) dan di optimasi untuk mengatasi kelemahan model dengan parameter kecil (misalnya 3b) yang mampu berjalan secara offline di hardware handphone.

---

## Kenapa Menggunakan Omyang? (Fitur Utama)

Omyang dibangun dengan filosofi "Spartan"—sangat ringan, tanpa beban berlebih, dan fokus pada eksekusi.

1. **Sangat Ringan (Single Static Binary):** Ditulis menggunakan Golang murni tanpa *dependency* pihak ketiga. Tidak butuh menginstal Node.js atau Python environment yang berat. Cukup satu file binary `omyang`, dan ia siap bekerja.
2. **Otak Ganda (Supervisor & Worker):** Omyang memecah tugas agar AI lokal yang kecil (seperti Llama 3B) tidak kebingungan. Ada agen "Orkestrator" yang bertugas memikirkan rencana besar, dan agen "Pekerja" yang bertugas mengeksekusi perintah bash/baca/tulis satu per satu tanpa banyak mikir.
3. **Ekosistem Otomatis (Self-Healing):** Cukup jalankan Omyang, ia akan otomatis membuat semua folder yang dibutuhkan (`models`, `skills`, `scripts`, `workspace`) dan file konfigurasi. Jika ada file setting yang rusak, Omyang bisa memperbaikinya sendiri tanpa *crash*.
4. **Hybrid Engine (Lokal & Cloud):** Kamu bisa menjalankan AI secara lokal menggunakan file `.gguf` di memori HP, atau langsung switch ke Cloud (Google Gemini) saat butuh pemikiran yang lebih berat, hanya dengan satu ketikan.
5. **Aman & Terkendali:** Omyang bekerja di dalam sandbox (`workspace/`). Perintah yang merusak sistem akan otomatis diblokir, dan kamu bisa mengatur agar Omyang selalu meminta izin (Y/N) sebelum mengeksekusi perintah di terminal.

---

## Cara Memulai (Instalasi dari Termux Kosong)

Untuk menjadikan Termux kamu sebagai pusaka, ikuti langkah berikut dari awal:

**1. Siapkan Lingkungan Termux**
Buka Termux dan pastikan sistem kamu up-to-date, lalu install Golang dan Git.
```bash
pkg update && pkg upgrade -y
pkg install golang git -y
```

**2. Clone dan Build Omyang**
Unduh kode sumber Omyang dan kompilasi menjadi binary.

```bash
git clone https://github.com/tonshans/omyang.git
cd omyang
go build -o omyang main.go
```

**3. Inisialisasi Ekosistem**
Jalankan Omyang untuk pertama kalinya agar ia membuat struktur folder secara otomatis.

```bash
./omyang
```

*Catatan: Pada tahap ini, Omyang akan meminta kamu meletakkan model AI (file .gguf) atau mengatur API Key Cloud.*

**4. Masukkan Model AI (Pilih salah satu)**

* **Cara Lokal:** Unduh model berformat `.gguf` (misal Llama-3.2-3B) dari HuggingFace, lalu pindahkan ke dalam folder `./models/`. Omyang memiliki fitur *Auto-Discovery* dan akan langsung mendeteksi model tersebut.
* **Cara Cloud:** Buka file `config.json` yang baru saja dibuat, masukkan API Key Gemini kamu pada bagian `cloud_api_key`, lalu simpan.

**5. Jalankan Omyang**

```bash
./omyang
```

Sekarang Omyang sudah aktif dan siap menerima perintahmu!

---

## Antarmuka dan Daftar Perintah (Slash Commands)

Omyang menggunakan tampilan teks yang bersih dengan **1-Row Dynamic Status Bar** di atas tempat kamu mengetik. Status bar ini menunjukkan mode mesin (Lokal/Cloud), mode kerja, mode keamanan, role yang dipakai, dan skill yang aktif.
Contoh tampilannya: `[ Local | Work | Copilot | sysadmin | termux_api ]`

Kamu mengendalikan Omyang dengan mengetik pesan biasa atau menggunakan perintah khusus (diawali garis miring `/`):

* `/talk` : Mode ngobrol santai. Omyang hanya akan menjadi teman diskusi tanpa bisa mengeksekusi perintah terminal.
* `/plan` : Mode perencanaan. Omyang akan membuat cetak biru langkah demi langkah untuk menyelesaikan tugasmu, tapi tidak akan mengeksekusinya.
* `/work` : Mode eksekusi sesungguhnya. Omyang akan membuat rencana lalu mendelegasikan tugas ke agen Pekerja untuk mengetik dan menjalankan perintah di terminal.
* `/mode <auto|copilot>` :
  * `copilot`: Omyang akan bertanya [y/n] kepadamu setiap kali mau menjalankan perintah bash.
  * `auto`: Omyang akan jalan sendiri (autopilot) dari awal sampai akhir.
* `/engine <local|cloud>` : Ganti mesin AI (dari GGUF lokal ke Gemini API, atau sebaliknya).
* `/role <nama_role>` : Ganti persona Omyang (misal: sysadmin, developer). Lihat daftarnya dengan mengetik `/role` saja.
* `/roleplay <deskripsi>` : Buat persona instan. Contoh: `/roleplay kamu adalah hacker linux senior dari tahun 90an`.
* `/skill <nama>` : Memuat modul pengetahuan ke otak Omyang.
* `/skill clear` : Menghapus skill yang sedang aktif.
* `/new` : Hapus ingatan percakapan saat ini dan mulai sesi baru (settingan engine dan role tetap utuh).
* `/resume` : Memulihkan sesi sebelumnya yang tersimpan secara otomatis.
* `/btw <pertanyaan>` : Tanya sesuatu di luar konteks obrolan (stateless query) tanpa mengganggu ingatan agen.
* `/exit` : Mematikan Omyang dan server lokal dengan aman.

---

## Cara Menambahkan Role, Skill, dan Script

Agar Omyang semakin pintar, kamu bisa memodifikasi pengetahuannya:

**1. Menambah Role (Persona)**
Buka file `roles.json`. Di dalamnya kamu bisa menambahkan profil baru dengan memberikan instruksi dasar (`system_prompt`) dan beberapa contoh obrolan (`few_shot`) agar Omyang tahu gaya bicara dan cara kerja persona tersebut. Setelah disimpan, panggil dengan `/role nama_role_baru`.

**2. Menambah Skill (Pengetahuan Domain)**
Buat file teks berakhiran `.md` (maksimal 4000 karakter) di dalam folder `./skills/`. Tuliskan panduan, dokumentasi API, atau SOP langkah kerja di dalamnya. Contoh: kamu membuat file `docker_guide.md`. Di dalam Omyang, cukup ketik `/skill docker_guide` agar Omyang otomatis jago tentang Docker.

**3. Menambah Script (Alat Bantu Cepat)**
Masukkan file bash script `.sh` atau python `.py` ke dalam folder `./scripts/`. Omyang bisa memanggil file ini saat berada di mode `/work` untuk mempercepat pekerjaannya tanpa harus mengetik ulang perintah yang panjang. Untuk mempermudah Omyang menggunakan script yang kamu buat, perkenalkan script ini dan gunanya di dalam skill yang kamu buat.

---

## Batas Kemampuan Omyang & Cara Mengatasinya

Omyang sangat tangguh, namun memiliki batas (terutama jika menggunakan AI lokal berukuran kecil). Berikut batasan dan trik mengatasinya:

**1. Hallusinasi dan Circular Looping (Berputar-putar di satu tempat)**

* *Masalah:* Kadang model AI lokal ukuran 3B kebingungan, mencoba menjalankan perintah yang sama berkali-kali namun gagal, lalu *stuck*.
* *Solusi:* Omyang punya *Circuit Breaker*. Jika dia mengulang hal yang sama lebih dari 2 kali, sistem akan memutus paksa prosesnya. Jika ini terjadi, atasi dengan mengetik `/engine cloud` untuk beralih ke model Gemini yang lebih pintar guna menyelesaikan tugas yang kompleks.

**2. Terkurung di dalam Sandbox (Workspace)**

* *Masalah:* Untuk keamanan, Omyang hanya bisa membaca dan menulis file di folder `./workspace/`. Dia tidak diizinkan mengubah sistem Termux kamu secara sembarangan.
* *Solusi:* Jika kamu ingin Omyang mengedit file di luar sandbox (misalnya file konfigurasi server di `/etc/`), gunakan perintah `bash` untuk melakukan `cp` (copy) dari file asli ke dalam workspace, suruh Omyang mengeditnya di workspace, lalu kamu (sebagai user) yang memindahkannya kembali ke sistem asli, atau izinkan dia menggunakan command pemindah jika mode keamanannya dipahami.

**3. Mengeksekusi Remote Command via SSH**

* *Masalah:* Omyang tidak bisa langsung paham cara membuka sesi interaktif SSH, karena input terminalnya akan tertahan (hanging).
* *Solusi:* Jangan menyuruh Omyang untuk "login SSH". Sebaliknya, buatlah sebuah file bash di dalam folder `./scripts/` (misal: `remote_exec.sh`) yang berisi perintah SSH satu jalan (contoh: `ssh user@ip "command"`). Lalu, panggil skill atau suruh Omyang untuk: *"Gunakan skrip remote_exec.sh untuk mengecek log di server target"*. Dengan cara ini, eksekusi berjalan satu kali (one-off) lalu Omyang akan membaca outputnya secara aman tanpa terjebak di sesi interaktif.

