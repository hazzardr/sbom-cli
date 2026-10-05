// Ingests every generated SBOM once, spread across concurrent VUs.
// Run via `mise run test:performance` (see run.sh); expects `sbom-cli serve`.
import http from 'k6/http';
import { check } from 'k6';
import { SharedArray } from 'k6/data';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const manifest = JSON.parse(open('./data/manifest.json'));
// SharedArray loads the documents once for all VUs instead of once per VU.
const docs = new SharedArray('sboms', () => manifest.files.map((f) => open(`./data/${f}`)));

export const options = {
  scenarios: {
    ingest: {
      executor: 'shared-iterations',
      vus: Number(__ENV.VUS || 4),
      iterations: docs.length,
      maxDuration: '15m',
    },
  },
  thresholds: {
    http_req_failed: ['rate==0'],
    checks: ['rate==1'],
  },
};

export default function () {
  const i = exec.scenario.iterationInTest;
  const res = http.post(`${BASE_URL}/sboms?source=${manifest.files[i]}`, docs[i], {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'ingest' },
  });
  check(res, {
    'stored as a new SBOM (201)': (r) => r.status === 201,
    // CycloneDX also indexes its metadata component, so >= rather than ==.
    'all components indexed': (r) => r.status === 201 && r.json('components') >= manifest.components,
  });
}
