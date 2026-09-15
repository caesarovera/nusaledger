// Load test transfer (docs/03 §7.3). Jalankan: k6 run test/load/transfer.js
// Env: BASE_URL (default http://localhost:8081 — port compose), USERS (default 50).
// SETELAH selesai WAJIB: GET /api/v1/internal/ledger/trial-balance → difference_sen == 0.
import http from 'k6/http';
import { check, fail } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '2m', target: 100 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<200', 'p(99)<500'],
    http_req_failed: ['rate<0.01'],
    checks: ['rate>0.99'],
  },
};

// 201 = transfer sukses, 422 = saldo kurang (sah, bukan kegagalan sistem)
http.setResponseCallback(http.expectedStatuses(201, 422));

const BASE = __ENV.BASE_URL || 'http://localhost:8081';
const USERS = Number(__ENV.USERS || 50);
const JSON_HEADERS = { 'Content-Type': 'application/json' };

export function setup() {
  const users = [];
  const run = Date.now();
  for (let i = 0; i < USERS; i++) {
    const email = `k6-${run}-${i}@load.local`;
    const reg = http.post(`${BASE}/api/v1/auth/register`, JSON.stringify({ email, password: 'rahasia123', full_name: `K6 ${i}` }), {
      headers: JSON_HEADERS, responseCallback: http.expectedStatuses(201),
    });
    const wallet = reg.json('data.wallet.public_id');
    const login = http.post(`${BASE}/api/v1/auth/login`, JSON.stringify({ email, password: 'rahasia123' }), {
      headers: JSON_HEADERS, responseCallback: http.expectedStatuses(200),
    });
    const token = login.json('data.access_token');
    if (login.status !== 200 || !token) {
      // Biasanya: rate limit login 5/15 menit per IP. Jalankan dengan docker-compose.load.yml.
      fail(`setup: login user ${i} gagal (${login.status}): ${login.body}`);
    }
    // saldo awal Rp 50.000.000 (maksimum per transaksi) — cukup untuk ribuan transfer @ Rp 11.000
    const topup = http.post(`${BASE}/api/v1/transactions/topup`, JSON.stringify({ amount_sen: 5000000000, description: 'seed k6' }), {
      headers: { ...JSON_HEADERS, Authorization: `Bearer ${token}`, 'Idempotency-Key': uuidv4() },
      responseCallback: http.expectedStatuses(201),
    });
    if (topup.status !== 201) fail(`setup: topup user ${i} gagal (${topup.status}): ${topup.body}`);
    users.push({ token, wallet });
  }
  return { users };
}

export default function (data) {
  const n = data.users.length;
  const i = Math.floor(Math.random() * n);
  let j = Math.floor(Math.random() * (n - 1));
  if (j >= i) j++;
  const from = data.users[i];
  const to = data.users[j];

  const res = http.post(
    `${BASE}/api/v1/transactions/transfer`,
    JSON.stringify({ to_account_public_id: to.wallet, amount_sen: 1000000, description: 'k6' }), // Rp 10.000
    { headers: { ...JSON_HEADERS, Authorization: `Bearer ${from.token}`, 'Idempotency-Key': uuidv4() }, tags: { name: 'transfer' } },
  );
  check(res, {
    'status 201': (r) => r.status === 201,
    'status 201 atau 422': (r) => r.status === 201 || r.status === 422,
    'ada transaction_id bila 201': (r) => r.status !== 201 || !!r.json('data.transaction_id'),
  });
}

export function teardown() {
  // verifikasi ledger dilakukan di luar k6 (butuh token ADMIN): lihat docs/evidence/k6.md
}
