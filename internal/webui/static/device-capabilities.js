/* Device-aware catalog policy. Conservative UHD detection keeps unsupported
 * 4K sources out of the catalog before playback can trigger server work. */
(function(){
  let snapshot={max_height:1080,video_codecs:[],hdr_known:false,hdr_types:[]};
  let pending=null;
  const failedUHD=new Set();
  const can=type=>{try{const video=document.createElement('video');return Boolean(video.canPlayType(type))}catch{return false}};
  const codecTypes={
    h264:'video/mp4; codecs="avc1.640033"',
    hevc:'video/mp4; codecs="hvc1.2.4.L153.B0"',
    av1:'video/mp4; codecs="av01.0.12M.10"',
    vp9:'video/webm; codecs="vp09.02.51.10"'
  };

  function displayHeight(){
    let css=Math.min(Number(screen?.width||innerWidth||0),Number(screen?.height||innerHeight||0));
    let height=Math.round(css*Math.max(1,Number(devicePixelRatio||1)));
    try{if(window.webapis?.productinfo?.isUdPanelSupported?.())height=Math.max(height,2160)}catch{}
    if(height>=2000)return 2160;
    if(height>=1300)return 1440;
    if(height>=900)return 1080;
    if(height>=650)return 720;
    return 480;
  }

  async function smooth4K(type){
    if(!navigator.mediaCapabilities?.decodingInfo)return false;
    try{
      let timer;
      const result=await Promise.race([
        navigator.mediaCapabilities.decodingInfo({type:'file',video:{contentType:type,width:3840,height:2160,bitrate:20000000,framerate:30}}),
        new Promise(resolve=>{timer=setTimeout(()=>resolve(null),800)})
      ]).finally(()=>clearTimeout(timer));
      return Boolean(result?.supported&&result?.smooth);
    }catch{return false}
  }

  async function detect(){
    const nativeQuery=window.StormFlixShell?.catalogQuery;
    if(typeof nativeQuery==='function'){
      try{
        const q=new URLSearchParams(String(nativeQuery.call(window.StormFlixShell)||'').replace(/^\?/,''));
        snapshot={max_height:Math.max(1080,Number(q.get('client_max_height')||1080)),video_codecs:(q.get('client_video_codecs')||'').split(',').filter(Boolean),hdr_known:false,hdr_types:[]};
        return snapshot;
      }catch{}
    }
    const playable=Object.entries(codecTypes).filter(([,type])=>can(type));
    const screenHeight=displayHeight();
    let maxHeight=1080;
    let codecs=playable.map(([codec])=>codec);
    if(screenHeight>=2000){
      const checks=await Promise.all(playable.map(async([codec,type])=>[codec,await smooth4K(type)]));
      const uhd=checks.filter(([,supported])=>supported).map(([codec])=>codec);
      if(uhd.length){maxHeight=2160;codecs=uhd}
    }
    snapshot={max_height:maxHeight,video_codecs:[...new Set(codecs)],hdr_known:false,hdr_types:[]};
    return snapshot;
  }

  function queryFrom(value){
    const q=new URLSearchParams();
    q.set('client_max_height',String(value.max_height||1080));
    if(value.video_codecs?.length)q.set('client_video_codecs',value.video_codecs.join(','));
    q.set('client_hdr_known','0');
    return`?${q.toString()}`;
  }

  window.sfDeviceCapabilitySnapshot=()=>({...snapshot,video_codecs:snapshot.video_codecs.filter(codec=>!failedUHD.has(codec))});
  window.sfRejectUHDCodec=codec=>failedUHD.add(String(codec||'').toLowerCase());
  // MediaCodec support is not HTMLMediaElement/container/audio-track support.
  // Native limits may only constrain the browser's actual playback contract.
  window.sfConstrainPlaybackCapabilities=(browser,native)=>{
    const limits=new Map((native?.video_profiles||[]).map(profile=>[profile.codec,profile]));
    const device=window.sfDeviceCapabilitySnapshot(),uhd=new Set(device.video_codecs);
    const profiles=(browser.video_codecs||[]).map(codec=>{
      const profile=limits.get(codec),height=failedUHD.has(codec)?1080:profile?Math.min(Number(profile.max_height)||1080,device.max_height):uhd.has(codec)?device.max_height:Math.min(device.max_height,1080);
      return {codec,max_width:profile?Math.min(Number(profile.max_width)||1920,height>=2000?4096:height>=1300?2560:1920):height>=2000?3840:height>=1300?2560:1920,max_height:height,max_frame_rate:profile?.max_frame_rate||(height>=2000?30:0),hdr_known:false,hdr_types:[]};
    });
    return {...browser,video_profiles:profiles};
  };
  window.sfCatalogCapabilityQuery=async()=>{
    if(!pending)pending=detect().catch(()=>snapshot);
    await pending;
    return queryFrom(window.sfDeviceCapabilitySnapshot());
  };
})();
