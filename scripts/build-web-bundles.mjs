import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import process from 'node:process';

const root=path.resolve(import.meta.dirname,'..');
const staticDir=path.join(root,'internal/webui/static');
const outputDir=path.join(staticDir,'bundles');
const check=process.argv.includes('--check');

const screens={
  music:{
    js:['music-ui.js'],
    css:['music.css','music-v2.css']
  },
  games:{
    js:['games-instant-cache.js','games-ui.js','games-player.js','games-g4-video.js','games-g4-session.js','games-g4.js','games-g4-polish.js','games-g43-ui.js','games-g44.js','games-g45-home-compat.js','games-g48-home.js','games-g49-collections.js'],
    css:['games.css','games-g3.css','games-g4.css','games-native-shell.css','games-g4-polish.css','games-g42.css','games-g43.css','games-g44.css','games-g48-home.css','games-g49-collections.css']
  }
};

function assemble(files,type){
  return files.map(file=>`/* StormFlix source: ${file} */\n${fs.readFileSync(path.join(staticDir,file),'utf8').trim()}\n`).join('\n');
}
function outputName(screen,type,body){
  const hash=crypto.createHash('sha256').update(body).digest('hex').slice(0,16);
  return `${screen}.${hash}.${type}`;
}

const generated=new Map();
const manifest={version:1,screens:{}};
for(const [name,definition] of Object.entries(screens)){
  const js=assemble(definition.js,'js');
  const css=assemble(definition.css,'css');
  const jsName=outputName(name,'js',js);
  const cssName=outputName(name,'css',css);
  generated.set(jsName,js);
  generated.set(cssName,css);
  manifest.screens[name]={js:`/bundles/${jsName}`,css:`/bundles/${cssName}`};
}
const manifestBody=`${JSON.stringify(manifest,null,2)}\n`;
generated.set('manifest.json',manifestBody);

if(check){
  for(const [name,body] of generated){
    const target=path.join(outputDir,name);
    if(!fs.existsSync(target)||fs.readFileSync(target,'utf8')!==body){
      throw new Error(`bundle desatualizado: ${path.relative(root,target)}`);
    }
  }
  const expected=new Set(generated.keys());
  for(const name of fs.readdirSync(outputDir)){
    if(!expected.has(name))throw new Error(`bundle obsoleto: ${path.relative(root,path.join(outputDir,name))}`);
  }
  process.stdout.write('Web screen bundles verificados.\n');
}else{
  fs.mkdirSync(outputDir,{recursive:true});
  for(const name of fs.readdirSync(outputDir))fs.rmSync(path.join(outputDir,name));
  for(const [name,body] of generated)fs.writeFileSync(path.join(outputDir,name),body);
  process.stdout.write(`Gerados ${generated.size-1} bundles de tela com conteúdo versionado.\n`);
}
