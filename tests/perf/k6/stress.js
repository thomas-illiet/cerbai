import http from "k6/http";
import { check, sleep } from "k6";

const PROXY_URL = __ENV.PROXY_URL || "http://cerbai:8085";
const PROXY_TOKEN = __ENV.PROXY_TOKEN || "";

export const options = {
  stages: [
    { duration: "2m", target: 50 },
    { duration: "3m", target: 100 },
    { duration: "2m", target: 200 },
    { duration: "1m", target: 0 },
  ],
  thresholds: {
    http_req_failed: ["rate<0.05"],
    http_req_duration: ["p(95)<2000"],
  },
};

const payload = JSON.stringify({
  model: "mock-model",
  messages: [{ role: "user", content: "ping" }],
});

const headers = {
  "Content-Type": "application/json",
  ...(PROXY_TOKEN ? { Authorization: `Bearer ${PROXY_TOKEN}` } : {}),
};

export default function () {
  const res = http.post(`${PROXY_URL}/v1/chat/completions`, payload, {
    headers,
  });
  check(res, {
    "status 200": (r) => r.status === 200,
  });
  sleep(0.2);
}
