// Runs the three query forms against the SBOMs loaded by ingest.js:
//   component, component + version, license.
// Run via `mise run test:performance` (see run.sh); expects `sbom-cli serve`.
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';

const BASE_URL = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const manifest = JSON.parse(open('./data/manifest.json'));

export const options = {
  scenarios: {
    query: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 8),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate==0'],
    checks: ['rate==1'],
    // No latency limits yet (no baseline). These always pass; they exist so
    // the summary reports latency per query form.
    'http_req_duration{name:component}': ['max>=0'],
    'http_req_duration{name:component_version}': ['max>=0'],
    'http_req_duration{name:license}': ['max>=0'],
  },
};

export default function () {
  const n = exec.scenario.iterationInTest;
  const c = manifest.samples[n % manifest.samples.length];
  const q = encodeURIComponent;

  switch (n % 3) {
    case 0: {
      const res = http.get(`${BASE_URL}/components?component=${q(c.name)}`, { tags: { name: 'component' } });
      check(res, { 'component found': (r) => r.status === 200 && r.json().length > 0 });
      break;
    }
    case 1: {
      const res = http.get(`${BASE_URL}/components?component=${q(c.name)}&version=${q(c.version)}`, {
        tags: { name: 'component_version' },
      });
      check(res, { 'component@version found': (r) => r.status === 200 && r.json().length > 0 });
      break;
    }
    default: {
      const license = manifest.licenses[n % manifest.licenses.length];
      const res = http.get(`${BASE_URL}/components?license=${q(license)}`, { tags: { name: 'license' } });
      // License results can be large; check the status without parsing the body.
      check(res, { 'license query ok': (r) => r.status === 200 });
    }
  }
}
