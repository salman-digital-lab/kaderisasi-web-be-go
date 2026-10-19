// Builds and runs the web API with ../env/<environment>/web-be-<environment>-env.
import {spawn,spawnSync} from 'node:child_process';
import {readFileSync,mkdirSync} from 'node:fs';
import {once} from 'node:events';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {parseEnv} from 'node:util';

const root=resolve(dirname(fileURLToPath(import.meta.url)),'..');
const environment=process.argv.find(arg=>arg.startsWith('--environment='))?.split('=')[1]??'test';
if(!['dev','test'].includes(environment))throw new Error('Environment must be dev or test');
const configured=parseEnv(readFileSync(resolve(root,'..',`env/${environment}/web-be-${environment}-env`),'utf8'));
const binary=resolve(root,'bin','web-api');
mkdirSync(resolve(root,'bin'),{recursive:true});
if(spawnSync('go',['build','-o',binary,'./cmd/web-api'],{cwd:root,stdio:'inherit'}).status!==0)process.exit(1);
const child=spawn(binary,[],{cwd:root,env:{...process.env,...configured,PORT:'3333'},stdio:'inherit'});
for(const signal of ['SIGTERM','SIGINT'])process.on(signal,()=>child.kill(signal));
const [code]=await once(child,'exit');process.exitCode=code??1;
