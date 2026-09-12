namespace BO.Services.Prague.Models;

public sealed partial class BOSportGameCache
{
    /// <summary>Remove all entries by iterating current values.</summary>
    public void Clear()
    {
        foreach (var item in Query().Execute())
        {
            Remove(item.EventId);
        }
    }
}