namespace Common.EventApi.Cache.Prague.Models;

public sealed partial class BPTicketTypeCache
{
    public void Clear()
    {
        foreach (var item in Query().Execute()) 
        {
            Remove(item.Key);
        }
    }
}