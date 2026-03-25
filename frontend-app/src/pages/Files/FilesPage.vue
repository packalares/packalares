<template>
  <div class="files-page">
    <div class="files-sidebar">
      <div class="sidebar-header"><q-icon name="sym_r_folder" size="22px" color="blue" /><span>Files</span></div>
      <q-list dense>
        <q-item v-for="f in sidebarFolders" :key="f.path" clickable :active="currentPath===f.path" active-class="sidebar-active" @click="navigateTo(f.path)" class="sidebar-item">
          <q-item-section avatar style="min-width:32px"><q-icon :name="'sym_r_' + f.icon" size="18px" /></q-item-section>
          <q-item-section><span class="sidebar-label">{{ f.label }}</span></q-item-section>
        </q-item>
      </q-list>
    </div>
    <div class="files-main">
      <div class="files-toolbar">
        <q-btn flat dense round icon="sym_r_arrow_back" size="sm" :disable="historyIdx<=0" @click="goBack" />
        <q-btn flat dense round icon="sym_r_arrow_forward" size="sm" :disable="historyIdx>=history.length-1" @click="goForward" />
        <q-btn flat dense round icon="sym_r_arrow_upward" size="sm" :disable="currentPath==='/'" @click="goUp" />
        <div class="breadcrumb">
          <span class="crumb" @click="navigateTo('/')">Root</span>
          <template v-for="(c,i) in breadcrumbs" :key="i">
            <span class="sep">/</span>
            <span class="crumb" @click="navigateTo(c.path)">{{c.name}}</span>
          </template>
        </div>
        <q-space />
        <q-btn flat dense round :icon="viewMode==='grid'?'sym_r_view_list':'sym_r_grid_view'" size="sm" @click="viewMode=viewMode==='grid'?'list':'grid'" />
        <q-btn flat dense round icon="sym_r_create_new_folder" size="sm" @click="showNewFolder=true" />
        <q-btn flat dense round icon="sym_r_upload_file" size="sm" @click="triggerUpload" />
        <q-btn flat dense round icon="sym_r_delete" size="sm" color="negative" :disable="selected<0 || !files[selected]" @click="deleteSelected" />
      </div>
      <div class="files-content" @click="selected=-1">
        <div v-if="viewMode==='grid'" class="file-grid">
          <div v-for="(file,idx) in files" :key="file.name" class="file-card" :class="{selected:selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)" @contextmenu.prevent.stop="onContext($event, file, idx)">
            <div class="file-icon-wrap" :class="file.isDir ? 'icon-folder' : 'icon-file'">
              <q-icon :name="file.isDir?'sym_r_folder':fileIcon(file.name)" size="28px" />
            </div>
            <div class="file-name">{{file.name}}</div>
            <div v-if="!file.isDir" class="file-meta">{{fmtSize(file.size)}}</div>
          </div>
        </div>
        <q-list v-else dense separator class="list-view">
          <q-item v-for="(file,idx) in files" :key="file.name" clickable :class="{'list-item-selected':selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)" @contextmenu.prevent.stop="onContext($event, file, idx)" class="list-item">
            <q-item-section avatar style="min-width:36px"><q-icon :name="file.isDir?'sym_r_folder':fileIcon(file.name)" :color="file.isDir?'amber':'grey'" size="20px" /></q-item-section>
            <q-item-section><span class="list-name">{{file.name}}</span></q-item-section>
            <q-item-section side style="min-width:80px;text-align:right"><span class="list-meta">{{file.isDir ? (file.numFiles||0)+' files' : fmtSize(file.size)}}</span></q-item-section>
            <q-item-section side style="min-width:130px"><span class="list-meta">{{fmtDate(file.modified)}}</span></q-item-section>
            <q-item-section side style="min-width:36px">
              <q-btn flat dense round icon="sym_r_delete" size="xs" color="negative" @click.stop="deleteFile(file)" />
            </q-item-section>
          </q-item>
        </q-list>
        <div v-if="!files.length&&!loading" class="empty-state"><q-icon name="sym_r_folder_open" size="48px" color="grey-7" /><div class="q-mt-sm">Empty folder</div></div>
      </div>
      <div class="files-status">
        {{files.length}} items
        <span v-if="selected>=0"> &mdash; {{files[selected]?.name}}</span>
        <span v-if="selected>=0 && !files[selected]?.isDir"> ({{fmtSize(files[selected]?.size)}})</span>
        <q-space />
        <span class="status-path">{{currentPath}}</span>
      </div>
    </div>
    <q-dialog v-model="showNewFolder"><q-card style="min-width:320px;border-radius:14px" class="bg-dark"><q-card-section class="text-subtitle1" style="font-weight:600">New Folder</q-card-section><q-card-section><q-input v-model="newFolderName" label="Folder name" dense dark outlined autofocus @keydown.enter="createFolder" /></q-card-section><q-card-actions align="right"><q-btn flat label="Cancel" v-close-popup class="btn-ghost" /><q-btn unelevated label="Create" class="btn-primary" @click="createFolder" /></q-card-actions></q-card></q-dialog>
    <input ref="uploadRef" type="file" multiple style="display:none" @change="handleUpload" />
  </div>
</template>
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';
import { api } from 'boot/axios';
import { useQuasar } from 'quasar';
const $q = useQuasar();
const currentPath = ref('/');
const files = ref<any[]>([]);
const loading = ref(false);
const viewMode = ref<'grid'|'list'>('grid');
const selected = ref(-1);
const history = ref(['/']);
const historyIdx = ref(0);
const showNewFolder = ref(false);
const newFolderName = ref('');
const uploadRef = ref<HTMLInputElement|null>(null);
const sidebarFolders = [
  {path:'/',label:'All Files',icon:'folder'},
  {path:'/Home',label:'Home',icon:'home'},
  {path:'/Home/Documents',label:'Documents',icon:'description'},
  {path:'/Home/Downloads',label:'Downloads',icon:'download'},
  {path:'/Home/Pictures',label:'Pictures',icon:'image'},
  {path:'/Home/Videos',label:'Videos',icon:'videocam'},
  {path:'/Home/Music',label:'Music',icon:'music_note'},
  {path:'/Mounts',label:'Mounts',icon:'lan'},
];
const breadcrumbs = computed(() => {
  const p = currentPath.value.split('/').filter(Boolean);
  return p.map((n,i) => ({name:n, path:'/'+p.slice(0,i+1).join('/')}));
});
async function loadFiles(path:string) {
  loading.value=true; selected.value=-1;
  try {
    const d:any = await api.get('/api/files/resources'+path);
    files.value = (d.items||[]).sort((a:any,b:any) => a.isDir!==b.isDir?(a.isDir?-1:1):a.name.localeCompare(b.name));
  } catch { files.value=[]; }
  loading.value=false;
}
function navigateTo(p:string) { currentPath.value=p; history.value=history.value.slice(0,historyIdx.value+1); history.value.push(p); historyIdx.value=history.value.length-1; loadFiles(p); }
function goBack() { if(historyIdx.value>0){historyIdx.value--;currentPath.value=history.value[historyIdx.value];loadFiles(currentPath.value);} }
function goForward() { if(historyIdx.value<history.value.length-1){historyIdx.value++;currentPath.value=history.value[historyIdx.value];loadFiles(currentPath.value);} }
function goUp() { if(currentPath.value==='/') return; const p=currentPath.value.split('/').filter(Boolean); p.pop(); navigateTo(p.length?'/'+p.join('/'):'/'); }
function openFile(f:any) { if(f.isDir) navigateTo(currentPath.value==='/'?'/'+f.name:currentPath.value+'/'+f.name); else window.open('/api/files/raw'+(currentPath.value==='/'?'':currentPath.value)+'/'+f.name,'_blank'); }
async function createFolder() { if(!newFolderName.value.trim()) return; try{await api.put('/api/files/resources'+currentPath.value+'/'+newFolderName.value.trim()+'/',{}); showNewFolder.value=false; newFolderName.value=''; loadFiles(currentPath.value);} catch{$q.notify({type:'negative',message:'Failed'});} }
function triggerUpload() { uploadRef.value?.click(); }
async function handleUpload(e:Event) { const i=e.target as HTMLInputElement; if(!i.files?.length) return; for(const f of Array.from(i.files)){const fd=new FormData();fd.append('file',f);try{await api.post('/api/files/resources'+currentPath.value+'/'+f.name,fd);}catch{}} i.value=''; loadFiles(currentPath.value); }
async function deleteFile(f:any) {
  $q.dialog({title:'Delete',message:`Delete ${f.name}?`,cancel:true,persistent:true,dark:true,ok:{label:'Delete',flat:true,color:'negative'}}).onOk(async()=>{
    try{await api.delete('/api/files/resources'+currentPath.value+'/'+f.name);loadFiles(currentPath.value);}catch{$q.notify({type:'negative',message:'Delete failed'});}
  });
}
function deleteSelected() { if(selected.value>=0&&files.value[selected.value]) deleteFile(files.value[selected.value]); }
function onContext(e:Event, f:any, idx:number) { selected.value=idx; }
function fileIcon(n:string) { const e=n.split('.').pop()?.toLowerCase()||''; const m:Record<string,string>={pdf:'sym_r_picture_as_pdf',doc:'sym_r_description',docx:'sym_r_description',txt:'sym_r_article',png:'sym_r_image',jpg:'sym_r_image',jpeg:'sym_r_image',gif:'sym_r_image',webp:'sym_r_image',svg:'sym_r_image',mp4:'sym_r_movie',mkv:'sym_r_movie',avi:'sym_r_movie',mp3:'sym_r_audio_file',flac:'sym_r_audio_file',wav:'sym_r_audio_file',zip:'sym_r_folder_zip',tar:'sym_r_folder_zip',gz:'sym_r_folder_zip','7z':'sym_r_folder_zip',rar:'sym_r_folder_zip',js:'sym_r_code',ts:'sym_r_code',py:'sym_r_code',go:'sym_r_code',html:'sym_r_code',css:'sym_r_code',json:'sym_r_code',yaml:'sym_r_code',yml:'sym_r_code',md:'sym_r_code',sh:'sym_r_terminal'}; return m[e]||'sym_r_draft'; }
function fmtSize(b:number) { if(!b)return''; const u=['B','KB','MB','GB','TB']; let i=0; while(b>=1024&&i<4){b/=1024;i++;} return b.toFixed(i>0?1:0)+' '+u[i]; }
function fmtDate(d:string) { return d?new Date(d).toLocaleDateString('en-US',{month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'}):''; }
onMounted(() => loadFiles(currentPath.value));
</script>
<style scoped lang="scss">
.files-page { display: flex; height: 100vh; background: var(--bg-1); }

.files-sidebar {
  width: 200px;
  background: var(--bg-1);
  border-right: 1px solid var(--separator);
  flex-shrink: 0;
  padding: 16px 8px;
}

.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px 14px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ink-1);
}

.sidebar-item {
  border-radius: 8px;
  min-height: 34px;
  margin-bottom: 1px;
}

.sidebar-active { background: var(--accent-soft) !important; color: var(--accent) !important; }

.sidebar-label { font-size: 13px; font-weight: 500; }

.files-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }

.files-toolbar {
  height: 44px;
  display: flex;
  align-items: center;
  gap: 2px;
  padding: 0 10px;
  border-bottom: 1px solid var(--separator);
  flex-shrink: 0;
}

.breadcrumb { display: flex; align-items: center; font-size: 13px; margin-left: 6px; }
.crumb { cursor: pointer; padding: 2px 6px; border-radius: 4px; color: var(--ink-2); font-weight: 500; &:hover { background: var(--glass); color: var(--ink-1); } }
.sep { margin: 0 1px; color: var(--ink-3); font-size: 11px; }

.files-content { flex: 1; overflow-y: auto; padding: 14px; }

.file-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 6px; }

.file-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 12px 6px 8px;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.1s ease;

  &:hover { background: var(--glass); }
  &.selected { background: var(--accent-soft); }
}

.file-icon-wrap {
  width: 42px;
  height: 42px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;

  &.icon-folder { background: rgba(251,191,36,0.10); color: #fbbf24; }
  &.icon-file { background: rgba(255,255,255,0.04); color: var(--ink-3); }
}

.file-name { font-size: 11px; margin-top: 6px; text-align: center; word-break: break-all; max-width: 86px; overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; color: var(--ink-1); font-weight: 500; }
.file-meta { font-size: 10px; color: var(--ink-3); margin-top: 2px; }

.list-view { padding: 0; }
.list-item { min-height: 38px; border-radius: 6px; margin: 0 4px; }
.list-item-selected { background: var(--accent-soft) !important; }
.list-name { font-size: 13px; font-weight: 500; color: var(--ink-1); }
.list-meta { font-size: 11px; color: var(--ink-3); font-family: 'SF Mono', monospace; }

.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 60px 0; color: var(--ink-3); font-size: 13px; }

.files-status {
  height: 26px;
  display: flex;
  align-items: center;
  padding: 0 14px;
  font-size: 11px;
  color: var(--ink-3);
  border-top: 1px solid var(--separator);
  flex-shrink: 0;
}

.status-path { font-family: 'SF Mono', monospace; font-size: 10px; }
</style>
