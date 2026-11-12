import {defineConfig} from '@playwright/test';
export default defineConfig({testDir:'./tests/e2e/plugins',testMatch:'install-recovery.spec.ts',workers:1,retries:0,timeout:45000,outputDir:'../tmp/plugin-install-tasks-20261112/browser/artifacts',reporter:'list',
 use:{baseURL:'http://127.0.0.1:39581',headless:true,trace:'retain-on-failure',launchOptions:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH?{executablePath:process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH}:{}},
 webServer:{command:'node scripts/plugin-install-browser-fixture.mjs',url:'http://127.0.0.1:39581/powerx-bridge-client.js?v=2',reuseExistingServer:false,timeout:30000,gracefulShutdown:{signal:'SIGTERM',timeout:10000}}});
