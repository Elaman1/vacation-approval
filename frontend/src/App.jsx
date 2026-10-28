import React, { useCallback, useEffect, useState } from 'react'
import keycloak from './auth'
import { api } from './api'

const statuses = {
  pending_balance: 'Проверка баланса',
  pending_manager: 'Ожидает руководителя',
  approved: 'Одобрено',
  auto_approved: 'Автоматически одобрено',
  rejected: 'Отклонено',
  needs_hr_review: 'Требуется HR',
  start_unconfirmed: 'Запуск процесса требует проверки',
}

export default function App() {
  const isEmployee = keycloak.hasRealmRole('employee')
  const isManager = keycloak.hasRealmRole('manager')
  const [requests, setRequests] = useState([])
  const [pending, setPending] = useState([])
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [info, setInfo] = useState('')

  const refresh = useCallback(async () => {
    setError('')
    try {
      if (isEmployee) setRequests(await api('/vacations'))
      if (isManager) setPending(await api('/manager/pending-vacations'))
    } catch (e) {
      setError(e.message)
    }
  }, [isEmployee, isManager])

  useEffect(() => { refresh() }, [refresh])

  async function submit(event) {
    event.preventDefault()
    setBusy(true)
    setError('')
    setInfo('')
    try {
      if (!startDate || !endDate || endDate < startDate) {
        throw new Error('Проверь даты отпуска')
      }
      const created = await api('/vacations', {
        method: 'POST',
        body: JSON.stringify({ start_date: startDate, end_date: endDate }),
      })
      setInfo(`Заявка №${created.id} создана`)
      setStartDate('')
      setEndDate('')
      await refresh()
    } catch (e) {
      setError(e.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="page">
      <header className="top">
        <div>
          <h1>Vacation Approval</h1>
          <p className="muted">Пользователь: {keycloak.tokenParsed?.preferred_username || 'unknown'}</p>
        </div>
        <button className="secondary" onClick={() => keycloak.logout({ redirectUri: location.origin })}>Выйти</button>
      </header>

      {error && <p role="alert" className="error">{error}</p>}
      {info && <p className="success">{info}</p>}

      {isEmployee && <>
        <section>
          <h2>Новая заявка</h2>
          <form onSubmit={submit}>
            <label>Начало отпуска <input type="date" required value={startDate} onChange={e => setStartDate(e.target.value)} /></label>
            <label>Окончание отпуска <input type="date" required min={startDate || undefined} value={endDate} onChange={e => setEndDate(e.target.value)} /></label>
            <button type="submit" disabled={busy}>{busy ? 'Создание...' : 'Отправить заявку'}</button>
          </form>
        </section>
        <section>
          <div className="title-line"><h2>Мои заявки</h2><button className="secondary" onClick={refresh}>Обновить</button></div>
          <div className="scroll"><table><thead><tr><th>ID</th><th>С</th><th>По</th><th>Дни</th><th>Статус</th></tr></thead>
          <tbody>{requests.map(req => <tr key={req.id}><td>#{req.id}</td><td>{req.start_date}</td><td>{req.end_date}</td><td>{req.days}</td><td>{statuses[req.status] || req.status}</td></tr>)}</tbody></table></div>
          {requests.length === 0 && <p className="muted">Заявок пока нет</p>}
        </section>
      </>}

      {isManager && <section>
        <div className="title-line"><h2>Заявки, ожидающие руководителя</h2><button className="secondary" onClick={refresh}>Обновить</button></div>
        <p>Это только список. Одобрение и отказ выполняются в Camunda Tasklist.</p>
        <a href="http://localhost:8081/camunda/app/tasklist/default/" target="_blank" rel="noreferrer">Открыть Tasklist</a>
        <div className="scroll"><table><thead><tr><th>ID</th><th>Сотрудник</th><th>Дни</th><th>Статус</th></tr></thead>
          <tbody>{pending.map(req => <tr key={req.id}><td>#{req.id}</td><td>#{req.employee_id}</td><td>{req.days}</td><td>{statuses[req.status] || req.status}</td></tr>)}</tbody></table></div>
        {pending.length === 0 && <p className="muted">Ожидающих заявок нет</p>}
      </section>}
      {!isEmployee && !isManager && <p className="error">У вас нет роли employee или manager.</p>}
    </main>
  )
}
