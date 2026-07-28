import { PieChart, Pie, Cell, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { CHART_COLORS } from '../chartColors';

const data = [
  { name: '分类A', value: 400 },
  { name: '分类B', value: 300 },
  { name: '分类C', value: 300 },
  { name: '分类D', value: 200 },
  { name: '分类E', value: 100 },
];

const COLORS = CHART_COLORS;

export function PieChartExample() {
  return (
    <ResponsiveContainer width="100%" height="100%">
      <PieChart>
        <Pie
          data={data}
          cx="50%"
          cy="50%"
          labelLine={true}
          outerRadius={80}
          fill={CHART_COLORS[0]}
          dataKey="value"
          label={({ name, percent }) => `${name}: ${(percent * 100).toFixed(0)}%`}
        >
          {data.map((_, index) => (
            <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip
          contentStyle={{
            backgroundColor: 'var(--color-bg-default)',
            borderColor: 'var(--color-border-default)',
            color: 'var(--color-text-primary)'
          }}
          formatter={(value: number) => [`${value}`, '数量']}
        />
        <Legend 
          layout="horizontal" 
          verticalAlign="bottom" 
          align="center"
          wrapperStyle={{ color: 'var(--color-text-primary)' }}
        />
      </PieChart>
    </ResponsiveContainer>
  );
}
