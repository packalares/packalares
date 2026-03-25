<template>
  <div class="files-page">
    <div class="files-sidebar">
      <div class="sidebar-header"><q-icon name="sym_r_folder" size="24px" color="blue" /><span>Files</span></div>
      <q-list dense>
        <q-item v-for="f in sidebarFolders" :key="f.path" clickable :active="currentPath===f.path" active-class="sidebar-active" @click="navigateTo(f.path)">
          <q-item-section avatar><q-icon :name="'sym_r_' + f.icon" size="20px" /></q-item-section>
          <q-item-section>{{ f.label }}</q-item-section>
        </q-item>
      </q-list>
    </div>
    <div class="files-main">
      <div class="files-toolbar">
        <q-btn flat dense icon="sym_r_arrow_back" :disable="historyIdx<=0" @click="goBack" />
        <q-btn flat dense icon="sym_r_arrow_forward" :disable="historyIdx>=history.length-1" @click="goForward" />
        <q-btn flat dense icon="sym_r_arrow_upward" :disable="currentPath==='/'" @click="goUp" />
        <div class="breadcrumb">
          <span class="crumb" @click="navigateTo('/')">Root</span>
          <template v-for="(c,i) in breadcrumbs" :key="i">
            <span class="sep">/</span>
            <span class="crumb" @click="navigateTo(c.path)">{{c.name}}</span>
          </template>
        </div>
        <q-space />
        <q-btn flat dense :icon="viewMode==='grid'?'sym_r_view_list':'sym_r_grid_view'" @click="viewMode=viewMode==='grid'?'list':'grid'" />
        <q-btn flat dense icon="sym_r_create_new_folder" @click="showNewFolder=true" />
        <q-btn flat dense icon="sym_r_upload_file" @click="triggerUpload" />
        <q-btn flat dense icon="sym_r_delete" color="negative" :disable="selected<0 || !files[selected]" @click="deleteSelected" />
      </div>
      <div class="files-content" @click="selected=-1">
        <div v-if="viewMode==='grid'" class="file-grid">
          <div v-for="(file,idx) in files" :key="file.name" class="file-card" :class="{selected:selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)" @contextmenu.prevent.stop="onContext($event, file, idx)">
            <q-icon :name="file.isDir?'sym_r_folder':fileIcon(file.name)" size="48px" :color="file.isDir?'amber':'grey'" />
            <div class="file-name">{{file.name}}</div>
            <div v-if="!file.isDir" class="file-meta">{{fmtSize(file.size)}}</div>
          </div>
        </div>
        <q-list v-else dense separator>
          <q-item v-for="(file,idx) in files" :key="file.name" clickable :class="{'bg-blue-10':selected===idx}" @click.stop="selected=idx" @dblclick="openFile(file)" @contextmenu.prevent.stop="onContext($event, file, idx)">
            <q-item-section avatar><q-icon :name="file.isDir?'sym_r_folder':fileIcon(file.name)" :color="file.isDir?'amber':'grey'" /></q-item-section>
            <q-item-section>{{file.name}}</q-item-section>
            <q-item-section side style="min-width:80px;text-align:right">{{file.isDir ? (file.numFiles||0)+' files' : fmtSize(file.size)}}</q-item-section>
            <q-item-section side style="min-width:140px">{{fmtDate(file.modified)}}</q-item-section>
            <q-item-section side style="min-width:40px">
              <q-btn flat dense round icon="sym_r_delete" size="sm" color="negative" @click.stop="deleteFile(file)" />
            </q-item-section>
          </q-item>
        </q-list>
        <div v-if="!files.length&&!loading" class="empty-state"><q-icon name="sym_r_folder_open" size="64px" color="grey-7" /><div>Empty folder</div></div>
      </div>
      <div class="files-status">
        {{files.length}} items
        <span v-if="selected>=0"> — {{files[selected]?.name}}</span>
        <span v-if="selected>=0 && !files[selected]?.isDir"> ({{fmtSize(files[selected]?.size)}})</span>
        <q-space />
        <span>{{currentPath}}</span>
      </div>
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
.files-page{display:flex;height:100vh;background:var(--bg-1)}
.files-sidebar{width:220px;background:var(--bg-1);border-right:1px solid var(--separator);flex-shrink:0;padding:16px 0}
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
