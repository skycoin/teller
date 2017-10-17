import { Routes } from '@angular/router';


import { HomeComponent } from './components/home/home.component';
import { AddressComponent } from './components/address/address.component';


export const AppRoutes: Routes = [
  {
    path: '',
    component: HomeComponent,
    data: { title: '' }
  },
  {
    path: 'address/:id',
    component: AddressComponent,
    data: { title: '' }
  },

]
