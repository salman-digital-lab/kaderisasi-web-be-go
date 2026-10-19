// Differential check for phase 7: with the proxy gone, unknown paths, wrong
// methods, trailing slashes, HEAD and CORS on errors must answer as Adonis.
//
//   node scripts/contract-routing.mjs
import { withContract, Comparison, learnerToken } from './contract-lib.mjs';

const ports = { adonis: 3356, go: 3357 };
const smtp = { SMTP_HOST: '127.0.0.1', SMTP_PORT: '2', SMTP_USERNAME: 'x', SMTP_PASSWORD: 'x', MAIL_FROM: 'noreply@contract.test', RESET_PASSWORD_URL: 'http://web.contract.test/reset' };
const comparison = new Comparison(ports, { headers: ['access-control-allow-origin', 'access-control-allow-credentials', 'cache-control'] });
const ok = await withContract('contract-routing', { ports, env: smtp }, async (ctx) => {
  await ctx.seed((q) => q(`INSERT INTO public_users (email, account_status, created_at) VALUES ('r@contract.test', 'active', now())`));
  await ctx.startServers();
  const origin = { origin: 'https://web.contract.test' };
  for (const [method, path] of [['GET', '/'], ['GET', '/v2'], ['GET', '/v2/unknown'], ['POST', '/v2/unknown/deep?x=1'], ['DELETE', '/v2/courses'], ['PATCH', '/v2/profiles'],
    ['PUT', '/v2/activities'], ['GET', '/v2/activities/'], ['GET', '/v2/clubs/'], ['GET', '/v2//activities'], ['GET', '/v2/activities//'], ['GET', '/v2/./activities'], ['GET', '/v2/x/../activities'], ['GET', '/v2/activities/open-act/'], ['GET', '/v2/activities?'], ['GET', '/health'], ['GET', '/health/'], ['POST', '/health'],
    ['GET', '/v2/%7Euser'], ['GET', '/v2/caf%C3%A9'], ['GET', '/V2/activities'], ['GET', '/v2/activities?page=1'], ['HEAD', '/v2/activities'], ['HEAD', '/v2/unknown']])
    await comparison.both(`${method} ${path}`, method, path, { headers: origin });
  await comparison.both('unknown with auth', 'GET', '/v2/nothing', { headers: { ...origin, authorization: `Bearer ${learnerToken(1)}` } });
  return comparison.report('contract-routing');
});
process.exitCode = ok ? 0 : 1;
