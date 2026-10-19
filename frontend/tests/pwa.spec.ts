import {test,expect,type Page} from "@playwright/test";
import {createServer} from "node:http";
import {readFile} from "node:fs/promises";
import {resolve,extname} from "node:path";
async function server(){
 let revision=1;const root=resolve("dist"),http=createServer(async(req,res)=>{
  const path=new URL(req.url||"/","http://localhost").pathname;
  if(path.startsWith("/api/")){res.setHeader("Content-Type","application/json");res.setHeader("Cache-Control","no-store");res.end(JSON.stringify(path.endsWith("/auth/me")?{id:1,username:"admin",is_superuser:true,permissions:[]}:path.endsWith("/settings")?{site_name:"VeloRay",default_locale:"en",secret:"private-api-value"}:["/api/nodes","/api/inbounds","/api/customers","/api/services"].includes(path)?[]:{}));return}
  if(path.startsWith("/sub/")){res.setHeader("Cache-Control","no-store");res.end("private-subscription-credential");return}
  const file=resolve(root,"."+path);if(!file.startsWith(root+"/")){res.writeHead(404).end();return}
  try{let raw:Buffer;try{raw=await readFile(file)}catch(e){if(extname(path))throw e;raw=await readFile(resolve(root,"index.html"))}if(path==="/service-worker.js"&&revision>1)raw=Buffer.from(raw.toString().replaceAll("veloray-static-v1",`veloray-static-v${revision}`));res.setHeader("Content-Type",({".js":"text/javascript",".css":"text/css",".svg":"image/svg+xml",".png":"image/png",".woff2":"font/woff2",".webmanifest":"application/manifest+json"} as Record<string,string>)[extname(file)]||"text/html");res.setHeader("Cache-Control","no-store");res.end(raw)}catch{res.writeHead(404).end()}
 });await new Promise<void>(done=>http.listen(0,"127.0.0.1",done));const address=http.address();if(!address||typeof address==="string")throw Error("missing test server");let closed=false;
 return {setRevision:(value:number)=>{revision=value},origin:`http://127.0.0.1:${address.port}`,close:async()=>{if(closed)return;closed=true;http.closeAllConnections();await new Promise<void>(done=>http.close(()=>done()))}};
}
async function login(page:Page,origin:string){await page.addInitScript(()=>localStorage.setItem("veloray-locale","en"));await page.goto(origin+"/settings");await page.evaluate(async()=>{await navigator.serviceWorker.ready;if(!navigator.serviceWorker.controller)await new Promise<void>(done=>navigator.serviceWorker.addEventListener("controllerchange",()=>done(),{once:true}))})}
test("PWA caches public assets, excludes API/subscriptions, and survives real server loss",async({page,context})=>{
 const host=await server();try{
  await login(page,host.origin);
  await page.evaluate(async()=>{await fetch("/veloray-logo.png");await fetch("/api/settings");await fetch("/sub/test-only-token?format=json");await fetch("/sub/test-only-token")});
  const cached=await page.evaluate(async()=>{const cache=await caches.open("veloray-static-v1");return (await cache.keys()).map(r=>new URL(r.url).pathname)});
  expect(cached).toContain("/offline.html");expect(cached).toContain("/veloray-logo.png");expect(cached.some(p=>p.startsWith("/api/")||p.startsWith("/sub/"))).toBe(false);
  // Closing the actual server also cuts off fetches initiated by the worker,
  // unlike page-only offline emulation in some Chromium/Playwright revisions.
  await page.evaluate(async()=>{const cache=await caches.open("veloray-static-v1");for(let i=0;i<70;i++)await cache.put(`/assets/synthetic-${i}.js`,new Response("test"));await fetch("/app-icon.svg")});
  expect(await page.evaluate(async()=> (await (await caches.open("veloray-static-v1")).keys()).length)).toBeLessThanOrEqual(64);
  await host.close();await context.setOffline(true);const navigation=await page.goto(host.origin+"/customers");expect(navigation?.fromServiceWorker()).toBe(true);await expect(page.getByRole("heading",{name:"VeloRay is offline",exact:true})).toBeVisible();expect(await page.content()).not.toContain("private-api-value");expect(await page.content()).not.toContain("private-subscription-credential");
 }finally{await host.close()}
});
test("PWA activation removes obsolete VeloRay caches and preserves other applications",async({page})=>{
 const host=await server();try{
  await page.goto(host.origin+"/offline.html");await page.evaluate(async()=>{await caches.open("veloray-static-obsolete");await caches.open("another-app-cache");await navigator.serviceWorker.register("/service-worker.js");await navigator.serviceWorker.ready});
  await expect.poll(()=>page.evaluate(()=>caches.keys())).not.toContain("veloray-static-obsolete");expect(await page.evaluate(()=>caches.keys())).toContain("another-app-cache");
  const manifest=await (await page.request.get(host.origin+"/manifest.webmanifest")).json();expect(manifest.scope).toBe("/");expect(manifest.icons[0].sizes).toContain("512x512");
  expect(await page.evaluate(()=>new Promise(resolve=>{const icon=new Image();icon.onload=()=>resolve([icon.naturalWidth,icon.naturalHeight]);icon.src="/app-icon.svg"}))).toEqual([512,512]);
 }finally{await host.close()}
});

test("PWA waits for explicit update acceptance before replacing the running app",async({page})=>{
 const host=await server();try{
  await login(page,host.origin);host.setRevision(2);await page.evaluate(async()=>{await (await navigator.serviceWorker.getRegistration())?.update()});
  const update=page.getByRole("button",{name:"Update app",exact:true});await expect(update).toBeVisible();expect(await page.evaluate(async()=>!!(await navigator.serviceWorker.getRegistration())?.waiting)).toBe(true);
  await update.click();await expect(update).toHaveCount(0);await expect.poll(()=>page.evaluate(()=>caches.keys())).toContain("veloray-static-v2");expect(await page.evaluate(()=>caches.keys())).not.toContain("veloray-static-v1");
 }finally{await host.close()}
});
