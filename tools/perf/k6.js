// k6 load test for the containerised policy-inference service.
// Neutral environment: the app runs in a distroless container, the load
// generator runs in the grafana/k6 container.
//
//   docker run --rm -i --add-host=host.docker.internal:host-gateway \
//     grafana/k6 run - < tools/perf/k6.js
//
// POLICY_SHAPE=complex switches the payload to a generated layered DAG
// (37 nodes, 186 edges, ~6 KB), mirroring internal/policy/policytest.
import http from 'k6/http';
import { check } from 'k6';

export const options = {
  scenarios: {
    warmup: { executor: 'constant-arrival-rate', rate: 50, timeUnit: '1s',
      duration: '10s', preAllocatedVUs: 10, maxVUs: 100 },
    measure: { executor: 'constant-arrival-rate', rate: 50, timeUnit: '1s',
      duration: '60s', preAllocatedVUs: 10, maxVUs: 100,
      startTime: '10s' },
  },
  thresholds: {
    http_req_failed: ['rate==0'],
    http_req_duration: ['p(90)<30'],
  },
};

const POLICY = 'digraph P {' +
  ' start [result=""];' +
  ' score [result="tier=gold"];' +
  ' age [result="approved=true"];' +
  ' review [result="approved=false,reason=review"];' +
  ' reject [result="approved=false,reason=denied"];' +
  ' start -> score [cond="score>=600 && active==true"];' +
  ' start -> reject [cond="active==false"];' +
  ' score -> age [cond="segment==\\"prime\\""];' +
  ' score -> review [cond="segment!=\\"prime\\" || score<700"];' +
  ' age -> reject [cond="age<18"];' +
  ' }';

// layeredPolicy mirrors policytest.Layered: layers tiers of width nodes, each
// node pointing at every node in the next tier. Edges into the first node of
// a tier carry cond "ok" (true for the input below), the rest carry "skip".
function layeredPolicy(layers, width) {
  const lines = ['digraph P {', ' start'];
  let prev = ['start'];
  for (let l = 1; l <= layers; l++) {
    const current = [];
    for (let w = 0; w < width; w++) {
      const node = `l${l}_${w}`;
      current.push(node);
      lines.push(` ${node} [result="v${l}_${w}=${w}"]`);
      for (const from of prev) {
        lines.push(` ${from} -> ${node} [cond="${w === 0 ? 'ok' : 'skip'}"]`);
      }
    }
    prev = current;
  }
  lines.push('}');
  return lines.join('\n');
}

const COMPLEX = {
  policy_dot: layeredPolicy(6, 6),
  input: { ok: true, skip: false },
};

const MEDIUM = {
  policy_dot: POLICY,
  input: { age: 20, score: 720, segment: 'prime', active: true },
};

const BODY = JSON.stringify(__ENV.POLICY_SHAPE === 'complex' ? COMPLEX : MEDIUM);

const URL = `${__ENV.BASE_URL || 'http://host.docker.internal:8080'}/infer`;

export default function () {
  const res = http.post(URL, BODY, { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'status 200': (r) => r.status === 200 });
}
