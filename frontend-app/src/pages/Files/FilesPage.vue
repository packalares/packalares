<template>
  <div class="files-page">
    <div class="files-sidebar">
      <div class="sidebar-header"><q-icon name="folder" size="24px" color="blue" /><span>Files</span></div>
      <q-list dense>
        <q-item v-for="f in sidebarFolders" :key="f.path" clickable :active="currentPath===f.path" active-class="sidebar-active" @click="navigateTo(f.path)">
          <q-item-section avatar><q-icon :name="f.icon" size="20px" /></q-item-section>
          <q-item-section>{{ f.label }}</q-item-section>
        </q-item>
      </q-list>
    </div>
    <div class="files-main">
      <div class="files-toolbar">
        <q-btn flat dense icon="arrow_back" :disable="historyIdx<=0" @click="goBack" />
        <q-btn flat dense icon="arrow_forward" :disable="historyIdx>=history.length-1" @click="goForward" />
        <q-btn flat dense icon="arrow_upward" @click="goUp" />
        <div class="breadcrumb"><span v-for="(c,i) in breadcrumbs" :key="i" class="crumb" @click="navigateTo(c.path)">{{c.name}}<span v-if="i<breadcrumbs.length-1" class="sep">/</span></span></div>
        <q-space />
        <q-btn flat dense :icon="viewMode==='grid'?'view_list':'grid_view'" @click="viewMode=viewMode==='grid'?'list':'grid'" />
        <q-btn flat dense icon="create_new_folder" @click="showNewFolder=true" />
        <q-btn flat dense icon="upload_file" @click="triggerUpload" />
      </div>
      <div class="files-content" @click="selected=-1">
        <div v-if="viewMode==='grid'" class="file-grid">
          <div v-for="(file,idx) in files" :key="file.name" class="file-card" :class="{selected:selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)">
            <q-icon :name="file.isDir?'folder':fileIcon(file.name)" size="48px" :color="file.isDir?'amber':'grey'" />
            <div class="file-name">{{file.name}}</div>
            <div v-if="!file.isDir" class="file-meta">{{fmtSize(file.size)}}</div>
          </div>
        </div>
        <q-list v-else dense separator>
          <q-item v-for="(file,idx) in files" :key="file.name" clickable :class="{'bg-blue-10':selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)">
            <q-item-section avatar><q-icon :name="file.isDir?'folder':fileIcon(file.name)" :color="file.isDir?'amber':'grey'" /></q-item-section>
            <q-item-section>{{file.name}}</q-item-section>
            <q-item-section side>{{file.isDir?'':fmtSize(file.size)}}</q-item-section>
            <q-item-section side style="min-width:140px">{{fmtDate(file.modified)}}</q-item-section>
          </q-item>
        </q-list>
        <div v-if="!files.length&&!loading" class="empty-state"><q-icon name="folder_open" size="64px" color="grey-7" /><div>Empty folder</div></div>
      </div>
      <div class="files-status">{{files.length}} items<span v-if="selected>=0"> — {{files[selected]?.name}}</span></div>
    </div>
    <q-dialog v-model="showNewFolder"><q-card style="min-width:300px" class="bg-dark"><q-card-section class="text-h6">New Folder</q-card-section><q-card-section><q-input v-model="newFolderName" label="Name" dense dark autofocus @keydown.enter="createFolder" /></q-card-section><q-card-actions align="right"><q-btn flat label="Cancel" v-close-popup /><q-btn flat label="Create" color="primary" @click="createFolder" /></q-card-actions></q-card></q-dialog>
    <input ref="uploadRef" type="file" multiple style="display:none" @change="handleUpload" />
  </div>
</template>
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';
import { useQuasar } from 'quasar';
const $q = useQuasar();
const currentPath = ref('/Home');
const files = ref<any[]>([]);
const loading = ref(false);
const viewMode = ref<'grid'|'list'>('grid');
const selected = ref(-1);
const history = ref(['/Home']);
const historyIdx = ref(0);
const showNewFolder = ref(false);
const newFolderName = ref('');
const uploadRef = ref<HTMLInputElement|null>(null);
const sidebarFolders = [
  {path:'/Home',label:'Home',icon:'home'},{path:'/Home/Documents',label:'Documents',icon:'description'},
  {path:'/Home/Downloads',label:'Downloads',icon:'download'},{path:'/Home/Pictures',label:'Pictures',icon:'image'},
  {path:'/Home/Videos',label:'Videos',icon:'videocam'},{path:'/Home/Music',label:'Music',icon:'music_note'},
];
const breadcrumbs = computed(() => {
  const p = currentPath.value.split('/').filter(Boolean);
  return p.map((n,i) => ({name:n, path:'/'+p.slice(0,i+1).join('/')}));
});
async function loadFiles(path:string) {
  loading.value=true; selected.value=-1;
  try { const d:any = await api.get('/api/resources'+path); files.value = (d.items||[]).sort((a:any,b:any) => a.isDir!==b.isDir?(a.isDir?-1:1):a.name.localeCompare(b.name)); }
  catch { files.value=[]; }
  loading.value=false;
}
function navigateTo(p:string) { currentPath.value=p; history.value=history.value.slice(0,historyIdx.value+1); history.value.push(p); historyIdx.value=history.value.length-1; loadFiles(p); }
function goBack() { if(historyIdx.value>0){historyIdx.value--;currentPath.value=history.value[historyIdx.value];loadFiles(currentPath.value);} }
function goForward() { if(historyIdx.value<history.value.length-1){historyIdx.value++;currentPath.value=history.value[historyIdx.value];loadFiles(currentPath.value);} }
function goUp() { const p=currentPath.value.split('/').filter(Boolean); if(p.length>1){p.pop();navigateTo('/'+p.join('/'));} }
function openFile(f:any) { if(f.isDir) navigateTo(currentPath.value+'/'+f.name); else window.open('/api/raw'+currentPath.value+'/'+f.name,'_blank'); }
async function createFolder() { if(!newFolderName.value.trim()) return; try{await api.put('/api/resources'+currentPath.value+'/'+newFolderName.value.trim()+'/',{}); showNewFolder.value=false; newFolderName.value=''; loadFiles(currentPath.value);} catch{$q.notify({type:'negative',message:'Failed'});} }
function triggerUpload() { uploadRef.value?.click(); }
async function handleUpload(e:Event) { const i=e.target as HTMLInputElement; if(!i.files?.length) return; for(const f of Array.from(i.files)){const fd=new FormData();fd.append('file',f);try{await api.post('/api/resources'+currentPath.value+'/'+f.name,fd);}catch{}} i.value=''; loadFiles(currentPath.value); }
function fileIcon(n:string) { const e=n.split('.').pop()?.toLowerCase()||''; const m:Record<string,string>={pdf:'picture_as_pdf',doc:'description',txt:'article',png:'image',jpg:'image',jpeg:'image',mp4:'movie',mp3:'audio_file',zip:'folder_zip',js:'code',ts:'code',py:'code',go:'code',html:'code'}; return m[e]||'draft'; }
function fmtSize(b:number) { if(!b)return''; const u=['B','KB','MB','GB']; let i=0; while(b>=1024&&i<3){b/=1024;i++;} return b.toFixed(i>0?1:0)+' '+u[i]; }
function fmtDate(d:string) { return d?new Date(d).toLocaleDateString('en-US',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}):''; }
onMounted(() => loadFiles(currentPath.value));
</script>
<style scoped lang="scss">
.files-page{display:flex;height:100vh;background:var(--bg-1)}
.files-sidebar{width:240px;background:var(--bg-1);border-right:1px solid var(--separator);flex-shrink:0;padding:16px 0}
.sidebar-header{display:flex;align-items:center;gap:10px;padding:0 20px 16px;font-size:16px;font-weight:600}
.sidebar-active{background:var(--accent-soft)!important;color:var(--accent)!important}
.files-main{flex:1;display:flex;flex-direction:column;min-width:0}
.files-toolbar{height:48px;display:flex;align-items:center;gap:4px;padding:0 12px;border-bottom:1px solid var(--separator);flex-shrink:0}
.breadcrumb{display:flex;align-items:center;font-size:13px;margin-left:8px}
.crumb{cursor:pointer;padding:2px 4px;border-radius:4px;&:hover{background:var(--glass)}}
.sep{margin:0 2px;color:var(--ink-3)}
.files-content{flex:1;overflow-y:auto;padding:16px}
.file-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(100px,1fr));gap:8px}
.file-card{display:flex;flex-direction:column;align-items:center;padding:12px 8px;border-radius:8px;cursor:pointer;&:hover{background:var(--glass)}&.selected{background:var(--accent-soft)}}
.file-name{font-size:12px;margin-top:6px;text-align:center;word-break:break-all;max-width:90px;overflow:hidden;text-overflow:ellipsis;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical}
.file-meta{font-size:10px;color:var(--ink-3);margin-top:2px}
.empty-state{display:flex;flex-direction:column;align-items:center;gap:12px;padding:60px 0;color:var(--ink-3)}
.files-status{height:28px;display:flex;align-items:center;padding:0 16px;font-size:12px;color:var(--ink-3);border-top:1px solid var(--separator);flex-shrink:0}
</style>
