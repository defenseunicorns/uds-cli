import { ChartPieSolid, EyeSolid, ServerSolid } from 'flowbite-svelte-icons';

export const routes = [
  {
    path: '/',
    name: 'Overview',
    icon: ChartPieSolid
  },
  {
    path: '/monitor',
    name: 'Monitor',
    icon: EyeSolid,
    children: [
      {
        path: '/monitor/pepr',
        name: 'Pepr'
      },
      {
        path: '/monitor/events',
        name: 'Events'
      }
    ]
  },
  {
    path: '/resources',
    name: 'Resources',
    icon: ServerSolid,
    children: [
      {
        path: '/resources/pods',
        name: 'Pods'
      },
      {
        path: '/resources/deployments',
        name: 'Deployments'
      },
      {
        path: '/resources/daemonsets',
        name: 'DaemonSets'
      },
      {
        path: '/resources/statefulsets',
        name: 'StatefulSets'
      },
      {
        path: '/resources/packages',
        name: 'Packages'
      },
      {
        path: '/resources/services',
        name: 'Services'
      }
    ]
  }
];
