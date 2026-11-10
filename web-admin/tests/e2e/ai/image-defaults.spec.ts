import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) for (const legacy of [false, true]) {
  test(`image defaults ${legacy ? 'invalid saved value' : 'new model'} (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    test.setTimeout(120_000);
    await page.setViewportSize(narrow ? { width:390,height:844 } : {width:1440,height:1000});
    const tenant='00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant=>{
      const expiresAt=Date.now()+3600_000;
      const token=`header.${btoa(JSON.stringify({exp:Math.floor(expiresAt/1000),tid:tenant}))}.fixture`;
      localStorage.setItem('powerx.session',JSON.stringify({version:1,status:'active',sessionId:'image-default-proof',revision:1,accessToken:token,refreshToken:'fixture',tokenType:'Bearer',expiresAt,scope:'access',tenantUuid:tenant}));
      localStorage.setItem('px_locale','zh');localStorage.setItem('welcome_guide_completed','true');
    },tenant);
    let profiles:any[]=legacy?[{modality:'image',provider:'openai',model:'gpt-image-2',defaults:{size:'1024x1024',quality:'standard',format:'png'}}]:[];
    const saves:any[]=[],errors:string[]=[];
    const operation=(old:boolean)=>({driver:'openai',parameters:{size:{type:'string',enum:['1024x1024'],default:'1024x1024'},quality:{type:'string',enum:old?['standard','hd']:['auto','low','medium','high'],default:old?'standard':'auto'},format:{type:'string',enum:['png','jpeg','webp'],default:'png'}}});
    page.on('pageerror',e=>errors.push(e.message));
    await page.routeWebSocket(/\/api\/ws/,socket=>socket.close());
    await page.route(url=>url.pathname.startsWith('/api/'),async route=>{
      const url=new URL(route.request().url()),path=url.pathname;
      if(path.includes('/_nuxt_icon/'))return route.fallback();
      let data:any={};
      if(path.endsWith('/setup/status'))data={configured:true,requires_login:true,install_status:'installed'};
      else if(path.endsWith('/me/context'))data={is_root:true,current_tenant_uuid:tenant,current_member_id:1,user:{id:1,display_name:'生图参数验收',status:1,is_root:true},members:[{tenant_uuid:tenant,tenant_name:'验收',member_id:1,is_admin:true}]};
      else if(path.endsWith('/menus'))data={menus:[{id:'ai',title:'模型配置',path:'/settings/ai',visible:true,origin:'system'}]};
      else if(path.endsWith('/agents/providers'))data={providers:[{id:'openai',name:'OpenAI',auth:{scheme:'bearer',fields:['api_key','base_url'],defaults:{base_url:'https://api.openai.com/v1'}}}]};
      else if(path.endsWith('/agents/settings/credentials'))data={env:'dev',credentials:[{provider:'openai',has_credentials:true,data:{base_url:''}}]};
      else if(path.endsWith('/agents/settings/profiles'))data={env:'dev',profiles};
      else if(path.endsWith('/agents/settings/active'))data={profile:profiles.find(p=>p.modality===url.searchParams.get('modality'))||null};
      else if(path.endsWith('/agents/models'))data={models:['gpt-image-2','dall-e-3'],catalog_source:'online',items:['gpt-image-2','dall-e-3'].map(id=>({id,operations:{image_generation:operation(id==='dall-e-3')}}))};
      else if(path.endsWith('/agents/settings/save')){
        const body=route.request().postDataJSON();saves.push(body);
        profiles=[{modality:'image',provider:'openai',model:body.image.model,defaults:{...body.image}}];data={ok:true};
      }
      else if(path.endsWith('/skills/source-policy'))data={allowlist:['builtin'],effective_source:'default'};
      return route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({code:200,data})});
    });
    await page.goto('/settings/ai?modality=image');
    const model=page.getByTestId('ai-model-select'),quality=page.getByTestId('image-quality');
    await expect(model).toContainText('gpt-image-2',{timeout:90_000});await expect(model).toBeEnabled();
    const backdrop=page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');if(await backdrop.isVisible())await backdrop.click({position:{x:380,y:100}});
    const choose=async (control:typeof model,value:string)=>{await control.scrollIntoViewIfNeeded();await control.click();await page.getByRole('option',{name:value,exact:true}).click();};
    if(legacy){
      await expect(page.getByTestId('image-parameter-error')).toContainText('quality=standard');
      await expect(quality).toContainText('standard');await expect(page.getByTestId('ai-save-settings')).toBeDisabled();
      expect(await page.getByTestId('image-parameters').evaluate(e=>e.scrollWidth-e.clientWidth)).toBeLessThanOrEqual(1);
      await page.getByTestId('image-parameter-error').evaluate(e=>e.scrollIntoView({block:'center'}));
      await page.screenshot({path:info.outputPath(`image-invalid-${narrow?'narrow':'desktop'}.png`),fullPage:true,animations:'disabled'});
      await choose(quality,'high');await expect(page.getByTestId('image-parameter-error')).toHaveCount(0);
    }else{await expect(quality).toContainText('auto');}
    await quality.scrollIntoViewIfNeeded();await quality.click();
    expect(await page.getByRole('option').allTextContents()).toEqual(['auto','low','medium','high']);await page.keyboard.press('Escape');
    await choose(model,'dall-e-3');await expect(quality).toContainText('standard');
    await quality.click();expect(await page.getByRole('option').allTextContents()).toEqual(['standard','hd']);await page.keyboard.press('Escape');
    await choose(model,'gpt-image-2');await expect(quality).toContainText(legacy?'high':'auto');
    await page.getByTestId('ai-save-settings').click();await expect.poll(()=>saves.length).toBe(1);
    expect(saves[0].image.quality).toBe(legacy?'high':'auto');
    await expect(page.getByText('图像生成 配置已更新',{exact:true})).toBeVisible();
    await page.reload();await expect(model).toContainText('gpt-image-2');await expect(model).toBeEnabled();
    if(await backdrop.isVisible())await backdrop.click({position:{x:380,y:100}});
    await expect(quality).toContainText(legacy?'high':'auto');
    expect(await page.getByTestId('image-parameters').evaluate(e=>e.scrollWidth-e.clientWidth)).toBeLessThanOrEqual(1);
    await quality.evaluate(e=>e.scrollIntoView({block:'center'}));
    await page.screenshot({path:info.outputPath(`image-${legacy?'legacy':'new'}-${narrow?'narrow':'desktop'}.png`),fullPage:true,animations:'disabled'});
    expect(errors).toEqual([]);
  });
}
