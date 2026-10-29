// k6 load test for the containerised policy-inference service.
// Neutral environment: the app runs in a distroless container, the load
// generator runs in the grafana/k6 container.
//
//   docker run --rm -i --add-host=host.docker.internal:host-gateway \
//     grafana/k6 run - < tools/perf/k6.js
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

const URL = `${__ENV.BASE_URL || 'http://host.docker.internal:8080'}/infer`;

export default function () {
  const res = http.post(URL, JSON.stringify({
    policy_dot: POLICY,
    input: { age: 20, score: 720, segment: 'prime', active: true },
  }), { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'status 200': (r) => r.status === 200 });
}
