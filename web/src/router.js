import Login from './pages/Login.vue'
import Dashboard from './pages/Dashboard.vue'
import Applications from './pages/Applications.vue'
import AppDetail from './pages/AppDetail.vue'
import AccessMatrix from './pages/AccessMatrix.vue'
import Campaigns from './pages/Campaigns.vue'
import CampaignDetail from './pages/CampaignDetail.vue'
import Import from './pages/Import.vue'
import Audit from './pages/Audit.vue'
import SopSearch from './pages/SopSearch.vue'
import Evidence from './pages/Evidence.vue'
import AccessReview from './pages/AccessReview.vue'

export const routes = [
  { path: '/login', component: Login },
  { path: '/', component: Dashboard },
  { path: '/applications', component: Applications },
  { path: '/applications/:id', component: AppDetail },
  { path: '/matrix', component: AccessMatrix },
  { path: '/access-review', component: AccessReview },
  { path: '/campaigns', component: Campaigns },
  { path: '/campaigns/:id', component: CampaignDetail },
  { path: '/import', component: Import, meta: { roles: ['admin'] } },
  { path: '/sop', component: SopSearch },
  { path: '/evidence', component: Evidence },
  { path: '/audit', component: Audit, meta: { roles: ['admin', 'readonly'] } },
]
