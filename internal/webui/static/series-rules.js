/* StormFlix series-specific presentation rules */
(function(){
  const baseSetupTheme=setupTheme;
  setupTheme=function(item){
    if(!item||item.media_type!=='series'){
      stopTheme();
      const button=$('#theme-toggle'),wrap=$('#theme-info-wrap');
      if(button)button.classList.add('hidden');
      if(wrap)wrap.classList.add('hidden');
      return;
    }
    baseSetupTheme(item);
  };

  const baseRenderHero=renderHero;
  renderHero=function(item){
    baseRenderHero(item);
    if(item?.entity_type==='series'&&item.series_id){
      $('#hero-play').onclick=async()=>{
        try{
          const data=await request(`/series/${encodeURIComponent(item.series_id)}`);
          for(const season of data.seasons||[]){
            if(season.episodes?.length){playMedia(season.episodes[0]);return}
          }
          openSeries(item.series_id);
        }catch{openSeries(item.series_id)}
      };
      $('#hero-more').onclick=()=>openSeries(item.series_id);
    }
  };
})();
